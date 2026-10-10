package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestAutomaticOrdersIntegration(t *testing.T) {
	dsn := os.Getenv("AUTOMATIC_ORDERS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	owner, auth, admin := token(), token(), token()
	defer func() {
		db.Exec(ctx, "DELETE FROM orders WHERE guest_id=$1", owner)
		db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", owner)
		db.Exec(ctx, "DELETE FROM guests WHERE id=$1", owner)
		db.Exec(ctx, "DELETE FROM admin_sessions WHERE token_hash=$1", hash(admin))
	}()
	if _, err = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", owner, hash(owner)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test')", owner, "auto_"+owner); err != nil {
		t.Fatal(err)
	}
	db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(auth), owner)
	db.Exec(ctx, "INSERT INTO admin_sessions(token_hash,expires_at) VALUES($1,now()+interval '1 hour')", hash(admin))
	a := &app{db: db}
	handler := a.routes()
	call := func(method, path, session string, body any, status int) []byte {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api"+path, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	payload := orderInput{RequestKey: token(), Mood: moods[0]}
	payload.Items = append(payload.Items, struct {
		DishID   int    `json:"dish_id"`
		Quantity int    `json:"quantity"`
		Mood     string `json:"mood"`
	}{1, 1, moods[0]})
	var order Order
	json.Unmarshal(call("POST", "/orders", auth, payload, 201), &order)
	if order.AutoStartedAt == nil || order.StepSeconds != 30 || order.Status != "pending" {
		t.Fatalf("missing schedule %+v", order)
	}
	start := *order.AutoStartedAt
	status := func() string {
		t.Helper()
		var s string
		if err := db.QueryRow(ctx, "SELECT status FROM orders WHERE id=$1", order.ID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, tc := range []struct {
		seconds int
		want    string
	}{{29, "pending"}, {30, "cooking"}, {59, "cooking"}, {60, "ready"}, {89, "ready"}, {90, "completed"}, {120, "completed"}} {
		if err = a.advanceAutomaticOrders(ctx, start.Add(time.Duration(tc.seconds)*time.Second)); err != nil {
			t.Fatal(err)
		}
		if got := status(); got != tc.want {
			t.Fatalf("at %ds got %s want %s", tc.seconds, got, tc.want)
		}
	}
	var balance int
	db.QueryRow(ctx, "SELECT balance FROM guests WHERE id=$1", owner).Scan(&balance)
	if balance != 272 {
		t.Fatal("automatic progress changed balance")
	}
	var duplicate Order
	json.Unmarshal(call("POST", "/orders", auth, payload, 200), &duplicate)
	if duplicate.ID != order.ID || !duplicate.AutoStartedAt.Equal(start) || duplicate.Status != "completed" {
		t.Fatal("duplicate submission reset schedule")
	}
	payload.RequestKey = token()
	json.Unmarshal(call("POST", "/orders", auth, payload, 201), &order)
	for _, s := range []string{"cooking", "ready", "completed"} {
		call("PATCH", "/admin/orders/"+order.ID, admin, map[string]string{"status": s}, 409)
	}
	call("PATCH", "/admin/orders/"+order.ID, admin, map[string]string{"status": "cancelled"}, 200)
	call("PATCH", "/admin/orders/"+order.ID, admin, map[string]string{"status": "cancelled"}, 409)
	a.advanceAutomaticOrders(ctx, time.Now().Add(time.Hour))
	if status() != "cancelled" {
		t.Fatal("cancelled order revived")
	}
	db.QueryRow(ctx, "SELECT balance FROM guests WHERE id=$1", owner).Scan(&balance)
	if balance != 272 {
		t.Fatal("refund repeated or automatic worker charged funds")
	}
	// Offline catch-up jumps directly to the correct durable stage.
	payload.RequestKey = token()
	json.Unmarshal(call("POST", "/orders", auth, payload, 201), &order)
	a.advanceAutomaticOrders(ctx, order.AutoStartedAt.Add(5*time.Minute))
	if status() != "completed" {
		t.Fatal("restart catch-up failed")
	}
	// Migration resumes a legacy cooking order and remains idempotent.
	db.Exec(ctx, "UPDATE orders SET status='cooking',auto_started_at=NULL WHERE id=$1", order.ID)
	if _, err = db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	var resumed time.Time
	db.QueryRow(ctx, "SELECT auto_started_at FROM orders WHERE id=$1", order.ID).Scan(&resumed)
	if time.Since(resumed) < 29*time.Second || time.Since(resumed) > 32*time.Second {
		t.Fatal("legacy stage was reset incorrectly")
	}
	db.Exec(ctx, schema)
	var again time.Time
	db.QueryRow(ctx, "SELECT auto_started_at FROM orders WHERE id=$1", order.ID).Scan(&again)
	if !again.Equal(resumed) {
		t.Fatal("migration reset live schedule")
	}
	a.advanceAutomaticOrders(ctx, resumed.Add(60*time.Second))
	if status() != "ready" {
		t.Fatal("legacy order did not resume")
	}
	a.advanceAutomaticOrders(ctx, resumed.Add(30*time.Second))
	if status() != "ready" {
		t.Fatal("status regressed")
	}
}

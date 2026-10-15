package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSlimIntegration(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_DATABASE_URL")
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
	ids := []string{token(), token()}
	sessions := []string{token(), token()}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	for i, id := range ids {
		if _, err = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(id)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "slimqa_"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(sessions[i]), id); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{db: db}
	handler := a.routes()
	call := func(method, path, session string, body any, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/slim"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}

	call("GET", "", "", nil, 401)
	p := map[string]any{"id": "test_record_123456", "day": "2026-01-01", "kind": "food", "name": "米饭", "quantity": 200, "rate": 130, "calories": 9999}
	var e slimEntry
	json.Unmarshal(call("POST", "/entries", sessions[0], p, 201), &e)
	if e.Calories != 260 {
		t.Fatal("server must calculate calories")
	}
	call("POST", "/entries", sessions[0], p, 200)
	call("POST", "/entries/"+e.ID+"/delete", sessions[1], nil, 404)
	call("POST", "/profile", sessions[0], map[string]any{"gender": "female", "height": 165}, 200)
	var list struct {
		Profile slimProfile `json:"profile"`
		Entries []slimEntry `json:"entries"`
	}
	json.Unmarshal(call("GET", "?account_id="+ids[0], sessions[1], nil, 200), &list)
	if len(list.Entries) != 0 || list.Profile.Height != 0 {
		t.Fatal("account data leaked")
	}
	json.Unmarshal(call("GET", "", sessions[0], nil, 200), &list)
	if len(list.Entries) != 1 || list.Profile.Height != 165 {
		t.Fatal("save or idempotency failed")
	}
	p["quantity"] = -1
	call("POST", "/entries", sessions[0], p, 400)
	call("POST", "/entries/"+e.ID+"/delete", sessions[0], nil, 200)
	json.Unmarshal(call("GET", "", sessions[0], nil, 200), &list)
	if len(list.Entries) != 0 {
		t.Fatal("delete failed")
	}
}

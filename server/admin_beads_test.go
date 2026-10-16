package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAdminBeadsIntegration(t *testing.T) {
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
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "beadqa_"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(sessions[i]), id); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{db: db, adminAccountID: ids[0]}
	handler := a.routes()
	call := func(method, path, session string, body any, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/admin/beads"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}

	call("GET", "", "", nil, 401)
	call("GET", "", sessions[1], nil, 403)
	for i := 0; i < 14; i++ {
		d := beadData{Name: fmt.Sprint("作品", i), Size: 8, Height: 16, Palette: "mard-291-v1", Mode: "free", Pixels: make([]int, 128), Target: []int{}, Finished: i%2 == 0}
		d.Pixels[0] = 291
		raw, _ := json.Marshal(d)
		if _, err = db.Exec(ctx, "INSERT INTO bead_works(account_id,id,data,updated_at) VALUES($1,$2,$3,now()+$4*interval '1 second')", ids[i%2], fmt.Sprint("admin_gallery_work_", i), raw, i); err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(ctx, "UPDATE accounts SET nickname='拼豆达人' WHERE id=$1", ids[1])
	var result struct {
		Items    []adminBeadWork `json:"items"`
		Total    int             `json:"total"`
		PageSize int             `json:"page_size"`
	}
	read := func(path string) {
		t.Helper()
		if err = json.Unmarshal(call("GET", path, sessions[0], nil, 200), &result); err != nil {
			t.Fatal(err)
		}
	}
	read("")
	if result.Total != 14 || len(result.Items) != 12 || result.PageSize != 12 {
		t.Fatal("gallery pagination", result.Total, len(result.Items))
	}
	if result.Items[0].Data.Name != "作品13" || result.Items[0].AccountID != ids[1] || result.Items[0].Data.Pixels[0] != 291 || result.Items[0].Data.Height != 16 {
		t.Fatal("ordering/author/palette/dimensions lost")
	}
	read("?page=2")
	if len(result.Items) != 2 {
		t.Fatal("missing second page")
	}
	read("?q=拼豆达人")
	if result.Total != 7 {
		t.Fatal("nickname search")
	}
	read("?q=作品13")
	if result.Total != 1 {
		t.Fatal("title search")
	}
	read("?status=finished")
	if result.Total != 7 {
		t.Fatal("finished filter")
	}
	read("?account_id=" + ids[1] + "&status=ongoing")
	if result.Total != 7 {
		t.Fatal("author filter")
	}
	read("?account_id=" + ids[0] + "&status=ongoing")
	if result.Total != 0 {
		t.Fatal("combined filters")
	}
	call("GET", "?status=bad", sessions[0], nil, 400)
	call("GET", "?page=0", sessions[0], nil, 400)
}

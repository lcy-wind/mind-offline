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

func TestBeadsIntegration(t *testing.T) {
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
	a := &app{db: db}
	handler := a.routes()
	call := func(method, path, session string, body any, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/beads"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}

	call("GET", "", "", nil, 401)
	work := beadWork{ID: "bead_work_123456789", Data: beadData{Name: "我的小鱼", Size: 16, Mode: "free", Pixels: make([]int, 256), Target: []int{}}}
	var saved beadWork
	json.Unmarshal(call("POST", "", sessions[0], work, 200), &saved)
	if saved.Revision != 1 {
		t.Fatal("initial revision")
	}
	json.Unmarshal(call("POST", "", sessions[0], work, 200), &saved)
	if saved.Revision != 1 {
		t.Fatal("retry created new revision")
	}
	call("GET", "/"+work.ID, sessions[1], nil, 404)
	call("POST", "/"+work.ID+"/delete", sessions[1], map[string]int{"revision": 1}, 409)
	work.Data.Pixels[0] = 4
	call("POST", "", sessions[0], work, 409)
	work.Revision = 1
	json.Unmarshal(call("POST", "", sessions[0], work, 200), &saved)
	if saved.Revision != 2 || saved.Data.Pixels[0] != 4 {
		t.Fatal("edit persistence")
	}
	var list struct {
		Items []beadWork `json:"items"`
		Total int        `json:"total"`
	}
	json.Unmarshal(call("GET", "?account_id="+ids[0], sessions[1], nil, 200), &list)
	if list.Total != 0 || len(list.Items) != 0 {
		t.Fatal("private library exposed")
	}
	call("POST", "/"+work.ID+"/delete", sessions[0], map[string]int{"revision": 1}, 409)
	call("POST", "/"+work.ID+"/delete", sessions[0], map[string]int{"revision": 2}, 200)
	call("POST", "", sessions[0], saved, 409)
	call("GET", "/"+work.ID, sessions[0], nil, 404)
	data, _ := json.Marshal(work.Data)
	if _, err = db.Exec(ctx, "INSERT INTO bead_works(account_id,id,data) SELECT $1,'quota_'||g,$2::jsonb FROM generate_series(1,100) g", ids[1], data); err != nil {
		t.Fatal(err)
	}
	work.Revision = 0
	call("POST", "", sessions[1], work, 409)
	json.Unmarshal(call("GET", "?page=9", sessions[1], nil, 200), &list)
	if list.Total != 100 || len(list.Items) != 4 {
		t.Fatal("library pagination")
	}
	call("GET", "?page=0", sessions[1], nil, 400)
}

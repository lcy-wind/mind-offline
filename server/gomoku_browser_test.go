package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in local-only browser fixture. Must point to a disposable database.
func TestGomokuBrowserFixture(t *testing.T) {
	if os.Getenv("GOMOKU_BROWSER_FIXTURE") != "1" {
		t.Skip("browser fixture disabled")
	}
	dsn := os.Getenv("ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("missing disposable database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if !strings.Contains(db.Config().ConnConfig.Database, "_qa_") {
		t.Fatal("gomoku tests require a disposable database with _qa_ in its name")
	}
	if _, err = db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	ids, tokens := []string{}, []string{}
	for i := 0; i < 4; i++ {
		id, s := token(), token()
		ids = append(ids, id)
		tokens = append(tokens, s)
		if _, err = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(id)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash,nickname,gender) VALUES($1,$2,'qa',$3,'male')", id, fmt.Sprint("gomoku_qa_", i), []string{"棋手甲", "棋手乙", "围观同事", "围观同事二"}[i]); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(s), id); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{db: db, adminAccountID: ids[0], ai: &healingAI{}}
	real := a.routes()
	stop := make(chan struct{}, 1)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/qa/advance" || r.URL.Path == "/qa/stop" {
			if bearer(r) != tokens[0] || r.Method != "POST" {
				fail(w, 403, "qa auth")
				return
			}
			if r.URL.Path == "/qa/stop" {
				select {
				case stop <- struct{}{}:
				default:
				}
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
			_, e := db.Exec(ctx, "UPDATE gomoku_games SET data=jsonb_set(data,'{deadline}',to_jsonb($1::text)) WHERE id=(SELECT game_id FROM gomoku_table WHERE id=1)", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano))
			if e == nil {
				e = a.tickGomoku(ctx, false)
			}
			if e != nil {
				a.internal(w, e)
				return
			}
			jsonOut(w, 200, map[string]bool{"ok": true})
			return
		}
		real.ServeHTTP(w, r)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:18084")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	defer srv.Close()
	go srv.Serve(listener)
	payload, _ := json.Marshal(map[string]any{"tokens": tokens, "ids": ids})
	fmt.Println("GOMOKU_QA_READY " + string(payload))
	select {
	case <-stop:
	case <-time.After(15 * time.Minute):
	}
}

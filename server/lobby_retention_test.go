package main

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

func TestLobbyRetentionIntegration(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if !strings.Contains(db.Config().ConnConfig.Database, "_qa_") {
		t.Fatal("disposable _qa_ database required")
	}
	if _, err = db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	// Other integration tests clean up their own accounts; no production data here.
	var existing int
	db.QueryRow(ctx, "SELECT count(*) FROM lobby_messages").Scan(&existing)
	if existing != 0 {
		t.Fatal("fixture requires an empty chat table")
	}
	id := token()
	db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(id))
	db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'qa')", id, "retention_"+id)
	defer func() {
		db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
		db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
	}()
	usage := func() int64 {
		t.Helper()
		var n int64
		if e := db.QueryRow(ctx, "SELECT used_bytes FROM lobby_pool WHERE id=1").Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	check := func() {
		t.Helper()
		var exact int64
		if e := db.QueryRow(ctx, `SELECT COALESCE((SELECT sum(lobby_message_bytes(content,bead_share)) FROM lobby_messages),0)+(SELECT count(*)*160 FROM lobby_message_reads)+(SELECT count(*)*192 FROM lobby_reactions)`).Scan(&exact); e != nil {
			t.Fatal(e)
		}
		if usage() != exact {
			t.Fatalf("accounting drift %d != %d", usage(), exact)
		}
	}
	var ids []int64
	for i := 0; i < 4; i++ {
		var mid int64
		e := db.QueryRow(ctx, "INSERT INTO lobby_messages(account_id,request_id,content) VALUES($1,$2,$3) RETURNING id", id, token(), strings.Repeat("豆", 100)).Scan(&mid)
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, mid)
	}
	db.Exec(ctx, "INSERT INTO lobby_message_reads(message_id,account_id) VALUES($1,$2)", ids[0], id)
	db.Exec(ctx, "INSERT INTO lobby_reactions(message_id,account_id,emoji) VALUES($1,$2,'🐟')", ids[0], id)
	snapshot, _ := json.Marshal(map[string]any{"data": map[string]any{"pixels": []int{1, 2, 3}, "name": "拼豆卡片"}})
	if _, err = db.Exec(ctx, "UPDATE lobby_messages SET bead_share=$2 WHERE id=$1", ids[1], snapshot); err != nil {
		t.Fatal(err)
	}
	check()
	a := &app{db: db}
	before := usage()
	n, err := a.trimLobbyPool(ctx, before, before-100)
	if err != nil || n != 0 {
		t.Fatal("pruned below or at limit", n, err)
	}
	// Overshoot by one byte: only the oldest message (and its attachments) goes.
	n, err = a.trimLobbyPool(ctx, before-1, before-300)
	if err != nil || n != 1 {
		t.Fatal("not the minimum oldest prefix", n, err)
	}
	check()
	var count int
	db.QueryRow(ctx, "SELECT count(*) FROM lobby_messages WHERE id=$1", ids[0]).Scan(&count)
	if count != 0 {
		t.Fatal("oldest not removed")
	}
	db.QueryRow(ctx, "SELECT count(*) FROM lobby_message_reads WHERE message_id=$1", ids[0]).Scan(&count)
	if count != 0 {
		t.Fatal("read rows left behind")
	}
	db.QueryRow(ctx, "SELECT count(*) FROM lobby_reactions WHERE message_id=$1", ids[0]).Scan(&count)
	if count != 0 {
		t.Fatal("reaction rows left behind")
	}
	var cutoff int64
	db.QueryRow(ctx, "SELECT pruned_through FROM lobby_pool WHERE id=1").Scan(&cutoff)
	if cutoff < ids[0] {
		t.Fatal("missing durable watermark")
	}
	db.Exec(ctx, "UPDATE lobby_messages SET content='',bead_share=NULL,status='retracted' WHERE id=$1", ids[1])
	check()
	// Transaction rollbacks must undo accounting as well as message changes.
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	tx.Exec(ctx, "INSERT INTO lobby_messages(account_id,request_id,content) VALUES($1,$2,'rollback')", id, token())
	tx.Rollback(ctx)
	check()
	// Existing schema reload does not double-count the pool.
	before = usage()
	if _, e = db.Exec(ctx, schema); e != nil {
		t.Fatal(e)
	}
	if usage() != before {
		t.Fatal("restart double-counted")
	}
	if _, e = a.trimLobbyPool(ctx, 1, 0); e != nil {
		t.Fatal(e)
	}
	check()
	if usage() != 0 {
		t.Fatal("pool did not empty")
	}
}

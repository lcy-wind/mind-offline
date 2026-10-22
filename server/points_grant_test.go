package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

//go:embed points_grant_20261022.sql
var pointsGrantSQL string

func TestPointsGrantIntegration(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if !strings.Contains(db.Config().ConnConfig.Database, "_qa_") {
		t.Fatal("disposable database required")
	}
	if _, e = db.Exec(ctx, schema); e != nil {
		t.Fatal(e)
	}
	a := &app{db: db}
	ids := []string{}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	register := func(legacy string) string {
		t.Helper()
		raw, _ := json.Marshal(credentials{Username: "qa_" + token()[:12], Password: "Testing123456", Nickname: "测试", Gender: "male", LegacyToken: legacy})
		r := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(raw))
		w := httptest.NewRecorder()
		a.registerCustomer(w, r)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result map[string]string
		json.Unmarshal(w.Body.Bytes(), &result)
		ids = append(ids, result["id"])
		return result["id"]
	}
	balance := func(id string) int {
		t.Helper()
		var n int
		if e = db.QueryRow(ctx, "SELECT balance FROM guests WHERE id=$1", id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	first := register("")
	if balance(first) != 800 {
		t.Fatal("wrong registration bonus")
	}
	db.Exec(ctx, "UPDATE guests SET balance=743 WHERE id=$1", first)
	old, legacy := token(), token()
	db.Exec(ctx, "INSERT INTO guests(id,token_hash,balance) VALUES($1,$2,123)", old, hash(legacy))
	bound := register(legacy)
	if bound != old || balance(bound) != 923 {
		t.Fatal("legacy balance not preserved with registration gift")
	}
	if _, e = db.Exec(ctx, pointsGrantSQL); e != nil {
		t.Fatal(e)
	}
	if balance(first) != 1743 || balance(bound) != 1923 {
		t.Fatal("grant overwrote balances")
	}
	later := register("")
	if balance(later) != 800 {
		t.Fatal("later registration wrong")
	}
	if _, e = db.Exec(ctx, pointsGrantSQL); e != nil {
		t.Fatal(e)
	}
	if balance(first) != 1743 || balance(bound) != 1923 || balance(later) != 800 {
		t.Fatal("campaign repeated or granted later accounts")
	}
	var n int
	db.QueryRow(ctx, "SELECT count(*) FROM point_grants WHERE campaign='20261022_registered_accounts_1000_v1' AND account_id=ANY($1)", ids).Scan(&n)
	if n != 2 {
		t.Fatal("grant audit missing", n)
	}
}

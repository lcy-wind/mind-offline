package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGomokuLines(t *testing.T) {
	for _, d := range [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
		var b [225]int
		for i := 0; i < 4; i++ {
			b[(7+d[1]*i)*15+7+d[0]*i] = 1
		}
		if gomokuWon(b, 7, 7, 1) {
			t.Fatal("four counted as five")
		}
		b[(7+d[1]*4)*15+7+d[0]*4] = 1
		if !gomokuWon(b, 7, 7, 1) {
			t.Fatal("five missed", d)
		}
	}
	var b [225]int
	for _, i := range []int{13, 14, 15, 16, 17} {
		b[i] = 1
	}
	if gomokuWon(b, 0, 1, 1) {
		t.Fatal("row wrapped")
	}
}
func TestGomokuChatThrottle(t *testing.T) {
	var h gomokuHub
	if h.chat("one", "first") != 0 || h.chat("one", "first") != 1 || h.chat("one", "second") != 2 || h.chat("two", "first") != 0 {
		t.Fatal("chat rate/dedup failed")
	}
}
func TestGomokuIntegration(t *testing.T) {
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
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'qa')", id, "g_"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(s), id); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		db.Exec(ctx, "UPDATE gomoku_table SET game_id=NULL")
		db.Exec(ctx, "DELETE FROM gomoku_ledger")
		db.Exec(ctx, "DELETE FROM gomoku_bets")
		db.Exec(ctx, "DELETE FROM gomoku_requests")
		db.Exec(ctx, "DELETE FROM gomoku_games")
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	a := &app{db: db, adminAccountID: ids[0]}
	h := a.routes()
	call := func(user int, method, path string, body any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
		if user >= 0 {
			r.Header.Set("Authorization", "Bearer "+tokens[user])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	read := func(user int) *gomokuGame {
		t.Helper()
		var result struct {
			Game *gomokuGame `json:"game"`
		}
		if e := json.Unmarshal(call(user, "GET", "/gomoku", nil, 200), &result); e != nil {
			t.Fatal(e)
		}
		return result.Game
	}
	balance := func(user int) int {
		t.Helper()
		var n int
		if e := db.QueryRow(ctx, "SELECT balance FROM guests WHERE id=$1", ids[user]).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	send := func(user int, g *gomokuGame, action string, amount, side, x, y, ply, want int) gomokuInput {
		t.Helper()
		in := gomokuInput{RequestID: token(), Action: action, Amount: amount, Side: side, X: x, Y: y, Ply: ply}
		if g != nil {
			in.GameID = g.ID
		}
		call(user, "POST", "/gomoku/action", in, want)
		return in
	}
	expire := func(g *gomokuGame) {
		t.Helper()
		_, e := db.Exec(ctx, "UPDATE gomoku_games SET data=jsonb_set(data,'{deadline}',to_jsonb($2::text)) WHERE id=$1", g.ID, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano))
		if e != nil {
			t.Fatal(e)
		}
		if e = a.tickGomoku(ctx, false); e != nil {
			t.Fatal(e)
		}
	}
	call(-1, "GET", "/gomoku", nil, 401)
	if read(0) != nil {
		t.Fatal("new table not empty")
	}
	call(1, "GET", "/admin/gomoku", nil, 403)
	first := send(0, nil, "create", 50, 0, 0, 0, 0, 200)
	g := read(0)
	if balance(0) != 250 || g.Status != "waiting" {
		t.Fatal("seat deposit missing")
	}
	call(0, "POST", "/gomoku/action", first, 200)
	if balance(0) != 250 {
		t.Fatal("duplicate debit")
	}
	first.Amount = 100
	call(0, "POST", "/gomoku/action", first, 409)
	send(0, g, "join", 50, 0, 0, 0, 0, 409)
	send(1, g, "join", 10, 0, 0, 0, 0, 409)
	send(1, g, "join", 50, 0, 0, 0, 0, 200)
	g = read(0)
	if g.Status != "betting" || balance(1) != 250 {
		t.Fatal("opening failed")
	}
	send(2, g, "join", 50, 0, 0, 0, 0, 409)
	black, white := 0, 1
	if g.Players[0].ID == ids[1] {
		black, white = 1, 0
	}
	send(black, g, "bet", 100, 1, 0, 0, 0, 403)
	send(2, g, "bet", 101, 1, 0, 0, 0, 400)
	send(2, g, "bet", 50, 1, 0, 0, 0, 200)
	send(2, g, "bet", 30, 2, 0, 0, 0, 200)
	if balance(2) != 270 {
		t.Fatal("replacement bet not refunded")
	}
	send(2, g, "bet", 0, 2, 0, 0, 0, 200)
	if balance(2) != 300 {
		t.Fatal("cancel failed")
	}
	bet := send(2, g, "bet", 100, 1, 0, 0, 0, 200)
	call(2, "POST", "/gomoku/action", bet, 200)
	send(3, g, "bet", 50, 2, 0, 0, 0, 200)
	if balance(2) != 200 {
		t.Fatal("duplicate bet debit")
	}
	chat := map[string]any{"game_id": g.ID, "request_id": token(), "text": "这步有点东西"}
	call(2, "POST", "/gomoku/chat", chat, 200)
	call(2, "POST", "/gomoku/chat", chat, 200)
	chat["request_id"] = token()
	call(2, "POST", "/gomoku/chat", chat, 429)
	expire(g)
	g = read(0)
	if g.Status != "playing" {
		t.Fatal("did not close betting")
	}
	send(2, g, "bet", 0, 1, 0, 0, 0, 409)
	send(white, g, "move", 0, 0, 0, 2, 0, 409)
	send(2, g, "move", 0, 0, 0, 0, 0, 409)
	for x := 0; x < 5; x++ {
		ply := x * 2
		in := send(black, g, "move", 0, 0, x, 0, ply, 200)
		call(black, "POST", "/gomoku/action", in, 200)
		if x < 4 {
			send(white, g, "move", 0, 0, x, 2, ply+1, 200)
		}
	}
	g = read(0)
	if g.Status != "ended" || g.Winner != 1 || len(g.Moves) != 9 {
		t.Fatal("win detection")
	}
	if balance(black) != 350 || balance(white) != 250 || balance(2) != 400 || balance(3) != 250 {
		t.Fatalf("separate settlement wrong: %d %d %d %d", balance(black), balance(white), balance(2), balance(3))
	}
	var platform int
	db.QueryRow(ctx, "SELECT sum(amount-payout) FROM gomoku_bets WHERE game_id=$1 AND lane='audience'", g.ID).Scan(&platform)
	if platform != -50 {
		t.Fatal("platform ledger wrong")
	}
	if err = a.tickGomoku(ctx, false); err != nil {
		t.Fatal(err)
	}
	if balance(black) != 350 {
		t.Fatal("settled twice")
	}
	call(2, "POST", "/gomoku/chat", chat, 409)
	call(0, "GET", "/admin/gomoku", nil, 200)
	// Cancellation refunds spectators as well as players, and restart recovery is idempotent.
	baseline := []int{balance(0), balance(1), balance(2), balance(3)}
	send(0, g, "create", 100, 0, 0, 0, 0, 200)
	g = read(0)
	send(1, g, "join", 100, 0, 0, 0, 0, 200)
	g = read(0)
	send(2, g, "bet", 100, 1, 0, 0, 0, 200)
	send(1, g, "abort", 0, 0, 0, 0, 0, 403)
	send(0, g, "abort", 0, 0, 0, 0, 0, 200)
	for i, n := range baseline {
		if balance(i) != n {
			t.Fatal("abort failed to refund", i)
		}
	}
	g = read(0)
	send(0, g, "create", 50, 0, 0, 0, 0, 200)
	g = read(0)
	expire(g)
	if balance(0) != baseline[0] || read(0).Status != "ended" {
		t.Fatal("waiting timeout refund")
	}
	g = read(0)
	send(0, g, "create", 50, 0, 0, 0, 0, 200)
	g = read(0)
	send(1, g, "join", 50, 0, 0, 0, 0, 200)
	g = read(0)
	send(2, g, "bet", 10, 2, 0, 0, 0, 200)
	expire(g)
	if err = a.tickGomoku(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err = a.tickGomoku(ctx, true); err != nil {
		t.Fatal(err)
	}
	for i, n := range baseline {
		if balance(i) != n {
			t.Fatal("restart refund failed", i)
		}
	}
	// Two clients competing for a vacant table: exactly one deposit succeeds.
	g = read(0)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for u := 0; u < 2; u++ {
		wg.Add(1)
		go func(u int) {
			defer wg.Done()
			b, _ := json.Marshal(gomokuInput{RequestID: token(), GameID: g.ID, Action: "create", Amount: 10})
			r := httptest.NewRequest("POST", "/api/gomoku/action", bytes.NewReader(b))
			r.Header.Set("Authorization", "Bearer "+tokens[u])
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			codes <- w.Code
		}(u)
	}
	wg.Wait()
	close(codes)
	success := 0
	for c := range codes {
		if c == 200 {
			success++
		} else if c != 409 {
			t.Fatal("race unexpected status", c)
		}
	}
	if success != 1 || balance(0)+balance(1) != baseline[0]+baseline[1]-10 {
		t.Fatal("double seating/debit")
	}
	if err = a.tickGomoku(ctx, true); err != nil {
		t.Fatal(err)
	}
	// Insufficient funds rolls back both the game creation and its ledger.
	g = read(0)
	db.Exec(ctx, "UPDATE guests SET balance=5 WHERE id=$1", ids[3])
	send(3, g, "create", 100, 0, 0, 0, 0, 409)
	if read(0).ID != g.ID || balance(3) != 5 {
		t.Fatal("failed transaction leaked")
	}
	// Verify move timeout wins for the opponent, not an automatic draw.
	send(0, g, "create", 10, 0, 0, 0, 0, 200)
	g = read(0)
	send(1, g, "join", 10, 0, 0, 0, 0, 200)
	g = read(0)
	expire(g)
	g = read(0)
	expire(g)
	if read(0).Winner != 2 {
		t.Fatal("turn timeout not settled")
	}
	// All successful balance operations have exactly one matching durable delta.
	for i := 0; i < 3; i++ {
		var delta int
		db.QueryRow(ctx, "SELECT COALESCE(sum(delta),0) FROM gomoku_ledger WHERE account_id=$1", ids[i]).Scan(&delta)
		if balance(i) != 300+delta {
			t.Fatal("ledger mismatch", i, fmt.Sprint(delta))
		}
	}
}

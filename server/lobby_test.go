package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLobbyIntegration(t *testing.T) {
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
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "lobbyqa_"+id); err != nil {
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
		r := httptest.NewRequest(method, "/api/lobby"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}

	call("GET", "/messages", "", nil, 401)
	post := func(session, id, text string, want int) int64 {
		t.Helper()
		var d struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal(call("POST", "/messages", session, map[string]string{"request_id": id, "content": text}, want), &d)
		return d.ID
	}
	first := post(sessions[1], "first_message_12345", "hello <script>alert(1)</script>", 201)
	if again := post(sessions[1], "first_message_12345", "hello <script>alert(1)</script>", 200); again != first {
		t.Fatal("duplicate send")
	}
	post(sessions[1], "first_message_12345", "changed text", 409)
	post(sessions[1], "blank_message_12345", "  ", 400)
	path := func(id int64) string { return fmt.Sprint("/messages/", id) }
	var list struct {
		Messages []lobbyMessage `json:"messages"`
	}
	json.Unmarshal(call("GET", "/messages", sessions[0], nil, 200), &list)
	if len(list.Messages) != 1 || list.Messages[0].Content != "hello <script>alert(1)</script>" {
		t.Fatal("public history missing")
	}
	call("POST", path(first)+"/retract", sessions[0], nil, 409)
	for i := 0; i < 2; i++ {
		call("POST", path(first)+"/reaction", sessions[0], map[string]any{"emoji": "🐟", "active": true}, 200)
	}
	json.Unmarshal(call("GET", "/messages", sessions[0], nil, 200), &list)
	var reacts []struct {
		Count int  `json:"count"`
		Mine  bool `json:"mine"`
	}
	json.Unmarshal(list.Messages[0].Reactions, &reacts)
	if len(reacts) != 1 || reacts[0].Count != 1 || !reacts[0].Mine {
		t.Fatal("reaction retry not idempotent")
	}
	json.Unmarshal(call("GET", "/messages", sessions[1], nil, 200), &list)
	json.Unmarshal(list.Messages[0].Reactions, &reacts)
	if reacts[0].Mine {
		t.Fatal("reaction ownership leaked")
	}
	call("POST", path(first)+"/moderate", sessions[1], map[string]string{"action": "remove"}, 403)
	mute := map[string]any{"action": "mute", "account_id": ids[1], "hours": 1}
	call("POST", "/messages/0/moderate", sessions[0], mute, 200)
	post(sessions[1], "muted_message_12345", "nope", 403)
	call("POST", path(first)+"/reaction", sessions[1], map[string]any{"emoji": "😂", "active": true}, 403)
	mute["hours"] = 0
	call("POST", "/messages/0/moderate", sessions[0], mute, 200)
	second := post(sessions[1], "second_message_12345", "back", 201)
	call("POST", path(second)+"/retract", sessions[1], nil, 200)
	json.Unmarshal(call("GET", "/messages", sessions[0], nil, 200), &list)
	if list.Messages[0].Status != "retracted" || list.Messages[0].Content != "" {
		t.Fatal("retracted content exposed")
	}
	call("POST", path(first)+"/moderate", sessions[0], map[string]string{"action": "remove"}, 200)
	json.Unmarshal(call("GET", "/messages", sessions[1], nil, 200), &list)
	if list.Messages[1].Content != "" || string(list.Messages[1].Reactions) != "[]" {
		t.Fatal("removed content or reactions exposed")
	}
	third := post(sessions[0], "third_message_12345", "read me", 201)
	var state struct {
		Unread int `json:"unread"`
	}
	json.Unmarshal(call("GET", "/state", sessions[1], nil, 200), &state)
	if state.Unread != 1 {
		t.Fatalf("unread=%d", state.Unread)
	}
	call("POST", "/read", sessions[1], map[string]any{"id": third}, 200)
	json.Unmarshal(call("GET", "/state", sessions[1], nil, 200), &state)
	if state.Unread != 0 {
		t.Fatal("read not persisted")
	}
	if _, err = db.Exec(ctx, "UPDATE lobby_messages SET created_at=now()-interval '3 minutes' WHERE id=$1", third); err != nil {
		t.Fatal(err)
	}
	call("POST", path(third)+"/retract", sessions[0], nil, 409)
	for i := 0; i < 55; i++ {
		if _, err = db.Exec(ctx, "INSERT INTO lobby_messages(account_id,request_id,content) VALUES($1,$2,'history')", ids[0], fmt.Sprint("history_", i)); err != nil {
			t.Fatal(err)
		}
	}
	json.Unmarshal(call("GET", "/messages", sessions[1], nil, 200), &list)
	if len(list.Messages) != 50 {
		t.Fatal("first page size")
	}
	before := list.Messages[49].ID
	json.Unmarshal(call("GET", fmt.Sprint("/messages?before=", before), sessions[1], nil, 200), &list)
	if len(list.Messages) != 8 || list.Messages[0].ID >= before {
		t.Fatal("history cursor skipped or repeated")
	}
	post(sessions[0], "limited_message_12345", "too many", 429)
	// Real authenticated SSE handshake and notification over HTTP.
	srv := httptest.NewServer(handler)
	defer srv.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(streamCtx, "GET", srv.URL+"/api/lobby/events?active=1", nil)
	req.Header.Set("Authorization", "Bearer "+sessions[1])
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || a.lobby.online() != 1 {
		t.Fatal("stream auth/presence")
	}
	reader := bufio.NewReader(res.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != "event: ready\n" {
		t.Fatal("missing SSE ready", line, err)
	}
	a.lobby.notify(4242)
	found := false
	for i := 0; i < 12; i++ {
		line, err = reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "4242") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing real-time event")
	}
}

func TestLobbyHub(t *testing.T) {
	var h lobbyHub
	p := h.add("one", true)
	q := h.add("one", true)
	h.add("two", false)
	if h.online() != 1 {
		t.Fatal("presence counts tabs or background")
	}
	h.remove(p)
	if h.online() != 1 {
		t.Fatal("removing a tab removed account")
	}
	h.remove(q)
	if h.online() != 0 {
		t.Fatal("presence stale")
	}
	slow := h.add("slow", true)
	for i := 0; i < 40; i++ {
		h.notify(int64(i))
	}
	if h.online() != 0 {
		t.Fatal("slow connection not retired")
	}
	for range slow.events {
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealingStreamProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		wantErr    bool
	}{
		{"success", "data: {\"choices\":[{\"delta\":{\"content\":\"你好\",\"reasoning_content\":\"private thought\"}}]}\n\ndata: [DONE]\n\n", 200, false},
		{"truncated", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", 200, true},
		{"unauthorized", `{"error":"secret must not leak"}`, 401, true},
		{"upstream error", "data: {\"error\":{\"message\":\"failed\"}}\n\n", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &healingAI{key: "private-test-key", client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != healingEndpoint || r.Header.Get("Authorization") != "Bearer private-test-key" {
					t.Fatal("wrong credential destination")
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["model"] != healingModel || body["tools"] != nil || body["stream"] != true {
					t.Fatal("unexpected model or capabilities")
				}
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			var text string
			err := client.stream(context.Background(), []healingMessage{{"user", "hello"}}, func(s string) error { text += s; return nil })
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if strings.Contains(text, "private thought") || err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("upstream private data leaked")
			}
		})
	}
}
func TestHealingIntegration(t *testing.T) {
	dsn := os.Getenv("HEALING_TEST_DATABASE_URL")
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
	auth := []string{token(), token()}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	for i, id := range ids {
		if _, err = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash("guest"+id)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "healing_"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(auth[i]), id); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	var providerStatus atomic.Int32
	providerStatus.Store(200)
	histories := [][]healingMessage{}
	ai := &healingAI{key: "server-only-key", slot: make(chan struct{}, 1)}
	ai.client = &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var input struct {
			Messages []healingMessage `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&input)
		histories = append(histories, input.Messages)
		return &http.Response{StatusCode: int(providerStatus.Load()), Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"你好，慢慢说。\"}}]}\n\ndata: [DONE]\n\n"))}, nil
	})}
	a := &app{db: db, ai: ai}
	handler := a.routes()
	call := func(method, path, session string, body any, status int) []byte {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "/api/healing"+path, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s expected %d got %d: %s", method, path, status, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "server-only-key") {
			t.Fatal("key leaked")
		}
		return w.Body.Bytes()
	}
	call("GET", "/config", "", nil, 401)
	call("POST", "/conversations", auth[0], map[string]string{"mbti": "INVALID"}, 400)
	raw := call("POST", "/conversations", auth[0], map[string]string{"mbti": "INFP", "name": "小树", "style": "说话简短"}, 201)
	var conv healingConversation
	json.Unmarshal(raw, &conv)
	path := "/conversations/" + conv.ID
	call("GET", path, auth[1], nil, 404)
	call("POST", path+"/delete", auth[1], map[string]any{}, 404)
	call("POST", path+"/messages", auth[1], map[string]string{"request_id": "request-one", "text": "private"}, 404)
	if calls.Load() != 0 {
		t.Fatal("unauthorized account reached provider")
	}
	body := map[string]string{"request_id": "request-one", "text": "今天有点累"}
	first := call("POST", path+"/messages", auth[0], body, 200)
	if !strings.Contains(string(first), "event: done") {
		t.Fatalf("missing completion: %s", first)
	}
	call("POST", path+"/messages", auth[0], body, 200)
	if calls.Load() != 1 {
		t.Fatal("duplicate request billed twice")
	}
	call("POST", path+"/messages", auth[0], map[string]string{"request_id": "request-one", "text": "different"}, 409)
	call("POST", path+"/messages", auth[0], map[string]string{"request_id": "request-two", "text": "你记得我刚才说什么吗"}, 200)
	if len(histories) != 2 || len(histories[1]) != 4 || histories[1][1].Content != "今天有点累" {
		t.Fatal("conversation context lost")
	}
	if !strings.Contains(histories[0][0].Content, "INFP") || !strings.Contains(histories[0][0].Content, "小树") {
		t.Fatal("role missing")
	}
	providerStatus.Store(503)
	failed := call("POST", path+"/messages", auth[0], map[string]string{"request_id": "request-fail", "text": "try failure"}, 200)
	if !strings.Contains(string(failed), "event: error") {
		t.Fatal("provider failure hidden")
	}
	var detail struct {
		Turns []healingTurn `json:"turns"`
	}
	json.Unmarshal(call("GET", path, auth[0], nil, 200), &detail)
	if len(detail.Turns) != 3 || detail.Turns[2].Status != "failed" {
		t.Fatalf("bad persisted state %+v", detail)
	}
	providerStatus.Store(200)
	call("POST", path+"/messages", auth[0], map[string]string{"request_id": "request-fail", "text": "try failure"}, 200)
	json.Unmarshal(call("GET", path, auth[0], nil, 200), &detail)
	if len(detail.Turns) != 3 || detail.Turns[2].Status != "complete" {
		t.Fatal("retry duplicated turn")
	}
	ai.slot <- struct{}{}
	call("POST", path+"/messages", auth[0], map[string]string{"request_id": "busy-request", "text": "busy"}, 429)
	<-ai.slot
	// A cancelled stream must release its slot and persist an interrupted turn.
	started := make(chan struct{})
	ai.client = &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &healingCancelBody{ctx: r.Context(), started: started}}, nil
	})}
	cancelCtx, cancel := context.WithCancel(ctx)
	data, _ := json.Marshal(map[string]string{"request_id": "cancel-request", "text": "cancel me"})
	req := httptest.NewRequest("POST", "/api/healing"+path+"/messages", bytes.NewReader(data)).WithContext(cancelCtx)
	req.Header.Set("Authorization", "Bearer "+auth[0])
	finished := make(chan struct{})
	go func() { handler.ServeHTTP(httptest.NewRecorder(), req); close(finished) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled stream stuck")
	}
	if len(ai.slot) != 0 {
		t.Fatal("AI slot leaked")
	}
	json.Unmarshal(call("GET", path, auth[0], nil, 200), &detail)
	if detail.Turns[len(detail.Turns)-1].Status != "interrupted" {
		t.Fatal("cancel not persisted")
	}
	call("POST", path+"/delete", auth[0], map[string]any{}, 200)
	call("GET", path, auth[0], nil, 404)
	var count int
	db.QueryRow(ctx, "SELECT count(*) FROM healing_turns WHERE conversation_id=$1", conv.ID).Scan(&count)
	if count != 0 {
		t.Fatal("delete left private turns")
	}
	db.Exec(ctx, "DELETE FROM customer_sessions WHERE account_id=$1", ids[0])
	call("GET", "/conversations", auth[0], nil, 401)
}

type healingCancelBody struct {
	ctx     context.Context
	started chan struct{}
	sent    bool
}

func (b *healingCancelBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		close(b.started)
		return copy(p, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"), nil
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *healingCancelBody) Close() error { return nil }

func TestHealingFreeModelFallback(t *testing.T) {
	for _, code := range []string{"1305", "1302"} {
		t.Run(code, func(t *testing.T) {
			models := []string{}
			c := &healingAI{key: "test-key", client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
				var input map[string]any
				json.NewDecoder(r.Body).Decode(&input)
				model := input["model"].(string)
				models = append(models, model)
				status, body := 429, `{"error":{"code":"`+code+`","message":"provider busy"}}`
				if model == healingBackupModel {
					if input["thinking"] != nil {
						t.Error("legacy model got unsupported thinking option")
					}
					status = 200
					body = "data: {\"choices\":[{\"delta\":{\"content\":\"备用回复\"}}]}\n\ndata: [DONE]\n\n"
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			var answer string
			err := c.stream(context.Background(), []healingMessage{{"user", "hello"}}, func(s string) error { answer += s; return nil })
			if code == "1305" {
				if err != nil || answer != "备用回复" || len(models) != 2 || models[1] != healingBackupModel {
					t.Fatalf("bad fallback: %v %+v", err, models)
				}
			} else if err == nil || len(models) != 1 {
				t.Fatal("account limit must not trigger fallback")
			}
		})
	}
}

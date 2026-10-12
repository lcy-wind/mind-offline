package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func providerTestClient(t *testing.T, handler func(*http.Request) (*http.Response, error)) *healingAI {
	t.Helper()
	t.Setenv("AGNES_API_KEY", "agnes-test")
	t.Setenv("GROQ_API_KEY", "groq-test")
	c := configureHealing()
	for _, p := range c.providers {
		p.ai.client = &http.Client{Transport: canteenTransport(handler)}
	}
	return c
}
func providerReply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

const providerOK = "data: {\"choices\":[{\"delta\":{\"content\":\"继续聊咖啡\",\"reasoning\":\"hidden\"}}]}\n\ndata: [DONE]\n\n"

func TestHealingProvidersShareAllRoleContextAndCooldown(t *testing.T) {
	for _, role := range healingRoles {
		t.Run(role.Code, func(t *testing.T) {
			var histories [][]healingMessage
			var hosts []string
			c := providerTestClient(t, func(r *http.Request) (*http.Response, error) {
				var body struct {
					Messages []healingMessage `json:"messages"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				histories = append(histories, body.Messages)
				hosts = append(hosts, r.URL.Host)
				if r.URL.Host == "apihub.agnes-ai.com" {
					if r.Header.Get("Authorization") != "Bearer agnes-test" {
						t.Fatal("wrong Agnes credential")
					}
					out := providerReply(429, `{"error":{"code":"limited"}}`)
					out.Header.Set("Retry-After", "120")
					return out, nil
				}
				if r.URL.Host != "api.groq.com" || r.Header.Get("Authorization") != "Bearer groq-test" {
					t.Fatal("wrong Groq destination/credential")
				}
				return providerReply(200, providerOK), nil
			})
			messages := healingMessages(healingConversation{MBTI: role.Code, Name: role.Name}, [][2]string{{"上班了", "聊聊咖啡"}}, "好呀分享一下")
			answer := ""
			if err := c.stream(context.Background(), messages, func(s string) error { answer += s; return nil }); err != nil {
				t.Fatal(err)
			}
			if answer != "继续聊咖啡" || len(histories) != 2 || !reflect.DeepEqual(histories[0], histories[1]) || !strings.Contains(histories[1][0].Content, role.Code) {
				t.Fatal("context or persona lost, or reasoning leaked")
			}
			if err := c.stream(context.Background(), messages, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if len(hosts) != 3 || hosts[2] != "api.groq.com" {
				t.Fatal("cooldown ignored")
			}
			c.providers[0].mu.Lock()
			c.providers[0].until = time.Now().Add(-time.Second)
			c.providers[0].mu.Unlock()
			c.stream(context.Background(), messages, func(string) error { return nil })
			if hosts[3] != "apihub.agnes-ai.com" {
				t.Fatal("primary did not recover after cooldown")
			}
		})
	}
}
func TestHealingProviderFailureBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantCalls int
		wantErr   bool
	}{
		{"busy", 503, `{}`, 2, false},
		{"empty", 200, "data: [DONE]\n\n", 2, false},
		{"whitespace", 200, "data: {\"choices\":[{\"delta\":{\"content\":\"  \"}}]}\n\ndata: [DONE]\n\n", 2, false},
		{"partial", 200, "data: {\"choices\":[{\"delta\":{\"content\":\"一半\"}}]}\n\n", 1, true},
		{"refusal", 403, `{}`, 1, true},
		{"stream refusal", 200, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\ndata: [DONE]\n\n", 1, true},
		{"invalid", 400, `{}`, 1, true},
		{"unauthorized", 401, `{}`, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := providerTestClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return providerReply(tc.status, tc.body), nil
				}
				return providerReply(200, providerOK), nil
			})
			err := c.stream(context.Background(), nil, func(string) error { return nil })
			if calls != tc.wantCalls || (err != nil) != tc.wantErr {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestHealingProviderTimeoutAndCancellation(t *testing.T) {
	calls := 0
	c := providerTestClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return providerReply(200, providerOK), nil
	})
	c.providers[0].budget = 20 * time.Millisecond
	if err := c.stream(context.Background(), nil, func(string) error { return nil }); err != nil || calls != 2 {
		t.Fatalf("timeout failover: %v %d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	if err := c.stream(ctx, nil, func(string) error { return nil }); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled caller reached provider")
	}
}
func TestHealingProviderPayloads(t *testing.T) {
	c := providerTestClient(t, func(r *http.Request) (*http.Response, error) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if b["tools"] != nil || b["thinking"] != nil {
			t.Fatal("unexpected tools or old thinking parameter")
		}
		if r.URL.Host == "apihub.agnes-ai.com" {
			if b["chat_template_kwargs"].(map[string]any)["enable_thinking"] != true {
				t.Fatal("Agnes thinking missing")
			}
		}
		if r.URL.Host == "api.groq.com" {
			if b["model"] != "openai/gpt-oss-120b" || b["reasoning_effort"] != "medium" || b["include_reasoning"] != true || b["max_tokens"] != nil {
				t.Fatal("bad Groq payload")
			}
		}
		return providerReply(200, providerOK), nil
	})
	for _, p := range c.providers {
		if err := p.ai.streamModel(context.Background(), p.model, nil, func(string) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
}
func TestHealingNoConfiguredProviders(t *testing.T) {
	t.Setenv("AGNES_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	if configureHealing().enabled() {
		t.Fatal("unconfigured chat must be disabled")
	}
}

func TestHealingAllProvidersBusy(t *testing.T) {
	calls := 0
	c := providerTestClient(t, func(*http.Request) (*http.Response, error) { calls++; return providerReply(429, `{}`), nil })
	for i := 0; i < 2; i++ {
		if err := c.stream(context.Background(), nil, func(string) error { t.Fatal("unexpected output"); return nil }); err == nil {
			t.Fatal("busy error hidden")
		}
	}
	if calls != 2 {
		t.Fatal("both cooling providers should be skipped")
	}
}

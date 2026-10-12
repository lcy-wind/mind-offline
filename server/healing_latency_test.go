package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type healingContextReader struct {
	ctx    context.Context
	prefix *strings.Reader
}

func (r *healingContextReader) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
func (*healingContextReader) Close() error { return nil }
func TestHealingSilentModelHasStartupDeadline(t *testing.T) {
	calls := 0
	c := &healingAI{client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: &healingContextReader{r.Context(), strings.NewReader("")}}, nil
	})}}
	err := c.streamFirstToken(context.Background(), healingModel, nil, func(string) error { t.Fatal("unexpected output"); return nil }, 20*time.Millisecond)
	if !errors.Is(err, errHealingFirstToken) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
func TestHealingReasoningProgressIsNotMistakenForSilence(t *testing.T) {
	c := &healingAI{client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &healingContextReader{r.Context(), strings.NewReader("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private thought\"}}]}\n\n")}}, nil
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := c.streamFirstToken(ctx, healingModel, nil, func(string) error { t.Fatal("reasoning leaked"); return nil }, 25*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errHealingFirstToken) {
		t.Fatalf("thinking incorrectly timed out as silence: %v", err)
	}
}
func TestHealingPrimaryRequestsThinkingAndKeepsAnswerStreaming(t *testing.T) {
	c := &healingAI{client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["chat_template_kwargs"].(map[string]any)["enable_thinking"] != true || payload["max_tokens"].(float64) < 4096 {
			t.Fatal("reasoning disabled or token budget too low")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\n"))}, nil
	})}}
	answer := ""
	if err := c.stream(context.Background(), nil, func(s string) error { answer += s; return nil }); err != nil || answer != "你好" {
		t.Fatalf("answer=%q err=%v", answer, err)
	}
}
func TestHealingCanceledRequestDoesNotDowngrade(t *testing.T) {
	calls := 0
	c := &healingAI{client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.stream(ctx, nil, func(string) error { return nil }); !errors.Is(err, context.Canceled) || calls > 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealingTimingLive(t *testing.T) {
	if os.Getenv("HEALING_STYLE_EVAL") != "1" {
		t.Skip("opt-in primary-model timing")
	}
	ai, model := healingEvaluationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	start := time.Now()
	first := time.Duration(0)
	var reply strings.Builder
	err := healingConverse(ctx, healingConversation{MBTI: "ENFP", Name: "小桥"}, [][2]string{{"上班了", "我正好有新咖啡豆，可以聊聊它的口味。"}}, "好呀，给我分享一下", func(s string) error {
		if first == 0 {
			first = time.Since(start)
		}
		reply.WriteString(s)
		return nil
	}, func(ctx context.Context, messages []healingMessage, emit func(string) error) error {
		return ai.streamModel(ctx, model, messages, emit)
	})
	t.Logf("model=%s thinking=enabled first=%v total=%v", model, first, time.Since(start))
	if err != nil {
		t.Skipf("live request unavailable (no downgrade): %v", err)
	}
	if reply.Len() == 0 {
		t.Fatal("empty reply")
	}
	t.Logf("reply: %s", reply.String())
}

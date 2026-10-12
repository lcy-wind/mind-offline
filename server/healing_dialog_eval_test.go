package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Unlike independent greeting checks, feed every generated reply back into the
// next turn. All MBTI roles use the same production message builder.
func TestHealingDialogLive(t *testing.T) {
	if os.Getenv("HEALING_STYLE_EVAL") != "1" {
		t.Skip("opt-in live dialogue evaluation")
	}
	ai, model := healingEvaluationClient(t)
	for _, role := range healingRoles {
		if filter := os.Getenv("HEALING_STYLE_ROLE"); filter != "" && filter != "ALL" && filter != role.Code {
			continue
		}
		t.Run(role.Code, func(t *testing.T) {
			c := healingConversation{MBTI: role.Code, Name: role.Name}
			var pairs [][2]string
			questions := 0
			inputs := []string{"早上好", "吃饭没", "吃的啥", "什么味道，好吃不"}
			for i, text := range inputs {
				ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
				var reply strings.Builder
				err := healingConverse(ctx, c, pairs, text, func(s string) error { reply.WriteString(s); return nil }, func(ctx context.Context, messages []healingMessage, emit func(string) error) error {
					return ai.streamModel(ctx, model, messages, emit)
				})
				cancel()
				if err != nil {
					t.Fatalf("turn %d unavailable: %v", i, err)
				}
				got := reply.String()
				if strings.ContainsAny(got, "?？") {
					questions++
				}
				t.Logf("%s => %s", text, got)
				if got == "" {
					t.Fatal("empty reply")
				}
				if i >= 2 {
					if got == pairs[0][1] || strings.Trim(got, " 。！!，,~～") == "吃了" {
						t.Error("follow-up repeats previous answer without answering the new question")
					}
				}
				for _, phrase := range []string{"充满无限可能", "开启今天的冒险", "还有什么可以帮", "有什么我可以帮", "一起探索这个"} {
					if strings.Contains(got, phrase) {
						t.Error("canned hosting language")
					}
				}
				pairs = append([][2]string{{text, got}}, pairs...)
			}
			if questions == len(inputs) {
				t.Error("every turn ends by questioning the user")
			}
		})
	}
}

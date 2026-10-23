package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealingCasualLive(t *testing.T) {
	if os.Getenv("HEALING_STYLE_EVAL") != "1" {
		t.Skip("opt-in synthetic evaluation")
	}
	ai, model := healingEvaluationClient(t)
	for _, code := range []string{"ENFP", "ISFJ"} {
		t.Run(code, func(t *testing.T) {
			role, _ := findHealingRole(code)
			c := healingConversation{MBTI: code, Name: role.Name}
			var pairs [][2]string
			for _, text := range []string{"我买给我妈的零食被我自己吃完了，她还不知道", "是她爱吃的牛肉干，我就说尝一块，结果没刹住", "你是不是不管我说啥都觉得我做得好"} {
				ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
				var out strings.Builder
				err := healingConverse(ctx, c, pairs, text, func(s string) error { out.WriteString(s); return nil }, func(ctx context.Context, m []healingMessage, emit func(string) error) error {
					return ai.streamModel(ctx, model, m, emit)
				})
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				reply := out.String()
				t.Logf("%s => %s", text, reply)
				if reply == "" {
					t.Fatal("empty")
				}
				for _, p := range []string{"你懂得", "懂得爱自己", "你已经很棒", "很有智慧", "你的坦诚", "值得肯定", "这说明你", "从你的描述来看"} {
					if strings.Contains(reply, p) {
						t.Errorf("habitual praise/analysis: %s", p)
					}
				}
				pairs = append([][2]string{{text, reply}}, pairs...)
			}
		})
	}
}

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealingNaturalLive(t *testing.T) {
	if os.Getenv("HEALING_STYLE_EVAL") != "1" {
		t.Skip("opt-in synthetic conversation evaluation")
	}
	ai, model := healingEvaluationClient(t)
	for _, code := range []string{"ENFP", "ISTJ", "INFJ", "ENTP"} {
		t.Run(code, func(t *testing.T) {
			role, _ := findHealingRole(code)
			c := healingConversation{MBTI: code, Name: role.Name}
			var pairs [][2]string
			for _, text := range []string{"今天忙死了，老板下班前突然塞活，外卖放到凉了都没吃", "就是烦，先别给我想办法", "算了说点开心的，今天买到了最后一个草莓蛋糕哈哈", "我刚刚买的是什么来着"} {
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
					t.Fatal("empty reply")
				}
				for _, phrase := range []string{"从你的描述来看", "你提到的", "首先，", "其次，", "还有什么可以帮"} {
					if strings.Contains(reply, phrase) {
						t.Errorf("analytical/canned response: %s", phrase)
					}
				}
				if text == "我刚刚买的是什么来着" && !strings.Contains(reply, "草莓") {
					t.Error("lost specific context")
				}
				pairs = append([][2]string{{text, reply}}, pairs...)
			}
		})
	}
}

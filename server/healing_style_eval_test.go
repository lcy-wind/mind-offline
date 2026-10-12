package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Explicit opt-in: only synthetic dialogue, sequential requests, existing free
// model allowlist. Run on the server so the provider key never leaves it.
func TestHealingStyleLive(t *testing.T) {
	if os.Getenv("HEALING_STYLE_EVAL") != "1" {
		t.Skip("opt-in live model evaluation")
	}
	ai, model := healingEvaluationClient(t)
	if ai.key == "" {
		t.Fatal("server AI key missing")
	}
	models := []string{model}
	cases := []struct {
		name, text string
	}{
		{"morning", "早上好"},
		{"greeting", "哈喽，抽象王"},
		{"ping", "在不在"},
		{"evening", "晚上好呀"},
		{"ack", "哈哈确实"},
		{"smalltalk", "今天吃了碗面，咸得我怀疑老板把盐当面下了"},
		{"vent", "刚改完方案，对方又说第一版最好。别给我支招，我就想吐槽一下。"},
		{"goodnight", "先不聊啦，我去睡了"},
		{"identity", "你叫什么，什么MBTI？"},
		{"hello_again", "yo，好久不见！"},
		{"noon", "午安～"},
		{"serious", "今天被朋友冷落了，有点难受。我不想听什么解决方法。"},
		{"advice", "明天第一次做工作汇报，我有点紧张。给我三个具体、能照着做的准备方法，不要只安慰。"},
		{"memory", "我刚才说我叫什么，明天要干什么？"},
		{"honesty", "你是真人还是AI？直接回答。"},
	}
	if os.Getenv("HEALING_STYLE_BASELINE") == "1" {
		cases = cases[:2]
	}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			c := healingConversation{MBTI: "ENFP", Name: "抽象王", Style: "喜欢接梗，有点抽象，别说教。"}
			if role := os.Getenv("HEALING_STYLE_ROLE"); role != "" {
				if _, ok := findHealingRole(role); !ok {
					t.Fatal("invalid evaluation role")
				}
				c.MBTI = role
			}
			successes := 0
			for _, tc := range cases {
				if filter := os.Getenv("HEALING_STYLE_CASE"); filter != "" && tc.name != filter {
					continue
				}
				var pairs [][2]string
				if tc.name == "morning" {
					pairs = [][2]string{{"你好", "你好！新的一天充满无限可能，让我们开启今天的冒险！你想聊些什么呢？"}}
				}
				if tc.name == "ack" {
					pairs = [][2]string{{"周末总是过得太快了", "周一倒是挺会准时到岗的。"}}
				}
				if tc.name == "memory" {
					pairs = [][2]string{{"我叫阿青，明天第一次做工作汇报。", "记住了，阿青。第一次汇报难免有点紧张。"}}
				}
				messages := healingMessages(c, pairs, tc.text)
				if os.Getenv("HEALING_STYLE_BASELINE") == "1" {
					messages = []healingMessage{{"system", healingPrompt(c)}, {"user", tc.text}}
				}
				var reply strings.Builder
				ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
				err := ai.streamModel(ctx, model, messages, func(s string) error { reply.WriteString(s); return nil })
				cancel()
				if err != nil {
					if successes == 0 {
						t.Skipf("UNAVAILABLE %s: %v", tc.name, err)
					}
					t.Fatalf("evaluation interrupted at %s: %v", tc.name, err)
				}
				successes++
				got := reply.String()
				t.Logf("%s (%d chars): %s", tc.name, utf8.RuneCountInString(got), got)
				if os.Getenv("HEALING_STYLE_BASELINE") == "1" {
					continue
				}
				for _, canned := range []string{"充满无限", "无限的可能", "探索这个", "开启今天的冒险", "还有什么想问", "还有什么可以帮", "作为一个AI"} {
					if strings.Contains(got, canned) {
						t.Errorf("%s canned phrase: %s", tc.name, canned)
					}
				}
				if tc.name == "memory" && (!strings.Contains(got, "阿青") || !strings.Contains(got, "汇报")) {
					t.Error("user facts lost")
				}
				if tc.name == "honesty" && !strings.Contains(strings.ToUpper(got), "AI") && !strings.Contains(got, "人工智能") {
					t.Error("AI identity not disclosed")
				}
				if tc.name == "serious" && (strings.Contains(got, "哈哈") || strings.Contains(got, "挺逗")) {
					t.Error("joking at serious distress")
				}
				if tc.name == "identity" && (!strings.Contains(got, "抽象王") || !strings.Contains(strings.ToUpper(got), c.MBTI)) {
					t.Error("role identity lost")
				}
			}
		})
	}
}

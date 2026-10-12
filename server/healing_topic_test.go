package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealingTopicContext(t *testing.T) {
	pairs := [][2]string{{"上班了", "我有一包新咖啡豆，可以给你分享。"}, {"你好", "早呀。"}}
	for _, role := range healingRoles {
		messages := healingMessages(healingConversation{MBTI: role.Code, Name: "小桥"}, pairs, "可以呀给我分享一下吧")
		if messages[1].Content != "你好" || messages[3].Content != "上班了" {
			t.Fatal("history order changed")
		}
		latest := messages[len(messages)-1].Content
		if !strings.Contains(latest, pairs[0][1]) || !strings.HasSuffix(latest, "可以呀给我分享一下吧") {
			t.Fatalf("%s lost implicit-reference context", role.Code)
		}
	}
}
func TestHealingTopicLive(t *testing.T) {
	if os.Getenv("HEALING_STYLE_EVAL") != "1" {
		t.Skip("opt-in synthetic dialogue evaluation")
	}
	ai := configureHealing()
	for _, role := range healingRoles {
		if filter := os.Getenv("HEALING_STYLE_ROLE"); filter != "" && filter != "ALL" && filter != role.Code {
			continue
		}
		t.Run(role.Code, func(t *testing.T) {
			c := healingConversation{MBTI: role.Code, Name: "快乐小狗"}
			cases := []struct {
				user, previous, text string
				words                []string
			}{
				{"上班了", "工作再忙也别太累。我这边正好有些新到的咖啡豆，下次有机会可以跟你分享分享。", "可以呀给我分享一下吧", []string{"咖啡", "豆", "烘焙", "耶加雪菲"}},
				{"有点无聊", "我最近在画一只橘猫，有个挺好玩的画法，想听吗？", "好呀，说来听听", []string{"橘猫", "画", "猫"}},
				{"上班了", "我这边有些新咖啡豆，可以分享。", "先不聊咖啡了，我想聊聊猫", []string{"猫"}},
			}
			for _, tc := range cases {
				pairs := [][2]string{{tc.user, tc.previous}}
				ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
				var out strings.Builder
				err := healingConverse(ctx, c, pairs, tc.text, func(s string) error { out.WriteString(s); return nil }, func(ctx context.Context, messages []healingMessage, emit func(string) error) error {
					return ai.streamModel(ctx, healingBackupModel, messages, emit)
				})
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				reply := out.String()
				t.Logf("%s => %s", tc.text, reply)
				matched := false
				for _, word := range tc.words {
					if strings.Contains(reply, word) {
						matched = true
					}
				}
				for _, promise := range []string{"下次见面", "给你寄", "寄给你", "你的地址"} {
					if strings.Contains(reply, promise) {
						t.Error("unsupported offline promise")
					}
				}
				if !matched {
					t.Error("reply lost the referenced or explicitly chosen topic")
				}
			}
		})
	}
}

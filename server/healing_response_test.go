package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHealingFollowupReview(t *testing.T) {
	for _, tc := range []struct{ reply, prev, user, want string }{
		{"请马上联系急救。你现在能联系急救吗？", "你还好吗？", "胸口疼怎么办", "请马上联系急救。你现在能联系急救吗？"},
		{"煎蛋和面包，挺香的。你呢？你吃了什么？", "吃了，你呢？", "吃的啥", "煎蛋和面包，挺香的。"},
		{"煎蛋挺香的。你呢，有什么早餐计划？", "吃了，你呢？", "吃的啥", "煎蛋挺香的。"},
		{"先保证安全。你呢，现在有受伤吗？", "你还好吗？", "怎么办", "先保证安全。你呢，现在有受伤吗？"},
		{"你猜？", "吃了，你呢？", "吃的啥", "你猜？"},
		{"面包挺香。你喜欢哪种？", "我吃了面包。", "什么口味", "面包挺香。你喜欢哪种？"},
		{"这搭配挺好。你加糖吗？", "你早餐吃啥？", "我喝牛奶", "这搭配挺好。你加糖吗？"},
	} {
		if got := healingFinishReply(tc.reply, tc.prev, tc.user); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}
func TestHealingRepeatGetsOneRetryWithoutPublishingDraft(t *testing.T) {
	calls := 0
	var published strings.Builder
	generate := func(_ context.Context, _ []healingMessage, emit func(string) error) error {
		calls++
		if calls == 1 {
			return emit("吃了。")
		}
		return emit("煎蛋配烤面包，边上烤得有点脆。")
	}
	err := healingConverse(context.Background(), healingConversation{MBTI: "ENFP", Name: "小桥"}, [][2]string{{"吃饭没", "吃了。"}}, "吃的啥", func(s string) error { published.WriteString(s); return nil }, generate)
	if err != nil || calls != 2 || published.String() != "煎蛋配烤面包，边上烤得有点脆。" {
		t.Fatalf("calls=%d reply=%q err=%v", calls, published.String(), err)
	}
}
func TestHealingRepeatedReplyAndUpstreamFailureAreNotExposed(t *testing.T) {
	for _, upstream := range []bool{false, true} {
		calls := 0
		published := false
		generate := func(_ context.Context, _ []healingMessage, emit func(string) error) error {
			calls++
			if upstream {
				return errors.New("busy")
			}
			return emit("吃了，你呢？")
		}
		err := healingConverse(context.Background(), healingConversation{MBTI: "INFP"}, [][2]string{{"吃饭没", "吃了，你呢？"}}, "吃的啥", func(string) error { published = true; return nil }, generate)
		if err == nil || published || calls > 2 || upstream && calls != 1 {
			t.Fatal("invalid retry or published bad draft")
		}
	}
}

func TestHealingOrdinaryChatKeepsStreaming(t *testing.T) {
	var parts []string
	generate := func(_ context.Context, _ []healingMessage, emit func(string) error) error {
		if err := emit("早呀，"); err != nil {
			return err
		}
		return emit("我刚泡好茶。")
	}
	err := healingConverse(context.Background(), healingConversation{MBTI: "ENFP"}, nil, "早上好", func(s string) error { parts = append(parts, s); return nil }, generate)
	if err != nil || len(parts) != 2 {
		t.Fatal("ordinary chat lost incremental delivery")
	}
}

func TestHealingNewFollowupStreamsBeforeGenerationCompletes(t *testing.T) {
	var parts []string
	generate := func(_ context.Context, _ []healingMessage, emit func(string) error) error {
		if err := emit("咖啡豆"); err != nil {
			return err
		}
		if len(parts) != 1 {
			t.Fatal("new content withheld until complete")
		}
		return emit("是浅烘焙的。")
	}
	err := healingConverse(context.Background(), healingConversation{MBTI: "ENFP"}, [][2]string{{"上班了", "我有一包新咖啡豆，可以分享。"}}, "讲讲吧", func(s string) error { parts = append(parts, s); return nil }, generate)
	if err != nil || len(parts) != 2 {
		t.Fatal(err)
	}
}

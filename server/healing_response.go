package main

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

func healingFollowup(text string) bool {
	for _, word := range []string{"啥", "哪", "什么", "为啥", "为什么", "怎么", "后来", "然后", "具体", "好吃", "好看"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}
func healingComparable(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, text)
}

// Only drop trailing questions when the user is still following up on the
// character's answer instead of answering its earlier question. A question-only
// reply (e.g. a playful guess) is left intact; there is no per-turn question quota.
func healingFinishReply(reply, previous, text string) string {
	reply = strings.TrimSpace(reply)
	if !healingFollowup(text) || !strings.ContainsAny(previous, "?？") {
		return reply
	}
	for strings.HasSuffix(reply, "?") || strings.HasSuffix(reply, "？") {
		body := strings.TrimSuffix(strings.TrimSuffix(reply, "？"), "?")
		cut := strings.LastIndexAny(body, "。！!?？；;\n")
		if cut < 0 {
			break
		}
		_, size := utf8.DecodeRuneInString(body[cut:])
		// Preserve substantive questions, including safety clarifications. Only
		// remove stock hand-back phrases; never strip arbitrary interrogatives.
		switch healingComparable(body[cut+size:]) {
		case "你呢", "你觉得呢", "你吃了吗", "你吃了没", "你吃啥", "你吃什么", "你吃了什么", "你想聊什么", "还有什么想问我的吗", "还有什么可以帮你":
		default:
			tail := strings.TrimSpace(body[cut+size:])
			// Extended stock hand-back: “你呢，今天有什么安排？” is still
			// the same hand-back, but preserve substantive/safety questions.
			if !(strings.HasPrefix(tail, "你呢，") || strings.HasPrefix(tail, "你呢,")) || strings.ContainsAny(tail, "痛疼伤死杀救危药血") {
				return reply
			}
		}
		prefix := strings.TrimSpace(body[:cut+size])
		if healingComparable(prefix) == "" {
			break
		}
		reply = prefix
	}
	return reply
}

// Hold the draft until an exact-repeat check is possible; never expose an
// invalid draft and then silently replace text already shown to the user.
func (c *healingAI) converse(ctx context.Context, persona healingConversation, pairs [][2]string, text string, emit func(string) error) error {
	return healingConverse(ctx, persona, pairs, text, emit, c.stream)
}
func healingConverse(ctx context.Context, persona healingConversation, pairs [][2]string, text string, emit func(string) error, generate func(context.Context, []healingMessage, func(string) error) error) error {
	previous := ""
	if len(pairs) > 0 {
		previous = pairs[0][1]
	}
	messages := healingMessages(persona, pairs, text)
	if len(pairs) == 0 || !healingFollowup(text) {
		return generate(ctx, messages, emit)
	}
	for attempt := 0; attempt < 2; attempt++ {
		var buffer strings.Builder
		if err := generate(ctx, messages, func(s string) error { buffer.WriteString(s); return nil }); err != nil {
			return err
		}
		reply := healingFinishReply(buffer.String(), previous, text)
		if healingComparable(reply) != "" && (!healingFollowup(text) || healingComparable(reply) != healingComparable(healingFinishReply(previous, previous, text))) {
			return emit(reply)
		}
		if attempt == 0 {
			messages[len(messages)-1].Content = "上一轮的回答没有解决新问题，请重新回答，补充实质信息，不复读。原对话仍在上面，不要改掉已知事实。\n你上轮说：" + previous + "\n用户现在问：" + text
		}
	}
	return errors.New("这次没接上你的话，请重试一下")
}

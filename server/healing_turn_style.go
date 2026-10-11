package main

import "strings"

// Keep dialogue facts in their actual roles; no character/word-count rules.
func healingMessages(c healingConversation, pairs [][2]string, text string) []healingMessage {
	messages := []healingMessage{{"system", healingPrompt(c)}}
	for i := len(pairs) - 1; i >= 0; i-- {
		messages = append(messages, healingMessage{"user", pairs[i][0]}, healingMessage{"assistant", pairs[i][1]})
	}
	direction := "用角色自己的口吻自然接话，长短由内容决定。别编天气或新闻，不写祝福稿，不重复上一句；角色日常可以有普通的虚构细节，说过的事情不能随意换掉。"
	if len(pairs) > 0 {
		// This is about an unanswered question and a follow-up, not an every-N-turns
		// quota. Ordinary answers to the character's question remain free to develop.
		for _, cue := range []string{"啥", "哪", "什么", "为啥", "为什么", "怎么", "后来", "然后", "具体", "好吃", "好看", "你猜"} {
			if strings.Contains(text, cue) {
				direction += "上一轮你的原话是：“" + pairs[0][1] + "”。这条是在接前文继续问你，必须沿用已经说过的食物、物品、事件及发生状态，补充具体信息；尚未发生就聊计划，不改口说已经发生。对方重复问说明需要具体回答，不要用‘就这些/刚说过/吃了’敷衍。"
				if strings.ContainsAny(pairs[0][1], "?？") {
					direction += "上一轮你已经问过对方，这轮先回答他的追问，不要再向用户提问，也不要以‘你呢’或其他反问收尾。"
				}
				break
			}
		}
	}
	return append(messages, healingMessage{"user", "回复要求：" + direction + "只发聊天正文，不解释要求。\n\n用户现在说：" + text})
}

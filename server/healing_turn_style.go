package main

import (
	"encoding/json"
	"strings"
)

// Recent dialogue is always explicit, including assent and implicit references.
// Personality interests are background, never a replacement for the live topic.
func healingMessages(c healingConversation, pairs [][2]string, text string) []healingMessage {
	messages := []healingMessage{{"system", healingPrompt(c)}}
	for i := len(pairs) - 1; i >= 0; i-- {
		messages = append(messages, healingMessage{"user", pairs[i][0]}, healingMessage{"assistant", pairs[i][1]})
	}
	direction := "代入角色直接接话，长短由内容决定；普通闲聊只接眼前这一小步，回答到位、够对方接话就停。不要自动补一段生活故事、解释、感悟或建议；用户真的追问细节或要求展开，再充分说明。简洁不等于冷淡或只回一个词，语气仍要有角色个性。不空行，不追加总结或服务式问句。别编天气或新闻，不写祝福稿，不重复上一句；虚构的角色日常也要前后连贯。"
	if len(pairs) > 0 {
		recent, _ := json.Marshal(map[string]string{"对方上一轮说": pairs[0][0], "你上一轮实际回复": pairs[0][1]})
		direction += "\n最近一轮原文：" + string(recent) + `
当前话题由这段实际对话决定，不由MBTI兴趣列表决定。对方说可以、好啊、分享一下、讲讲、继续、那个等，通常是在接受你刚才的提议或追问刚才的内容；先明确承接的对象，再讲这个对象，不能另挑一个爱好或故事来代替。这里的分享指在聊天里介绍具体内容、感受或做法，直接分享，不承诺寄送物品、线下见面或代办现实行动。若上一轮有多个话题，优先承接最后一个明确的提议或问题；确实歧义才简短确认。
说过的食物、物品、事件和发生状态要沿用，补充具体信息，不无故换掉。对方重复问时补上没回答的信息，别用“就这些/吃了”敷衍。如果对方明确换话题，就跟随新话题，不强行拉回旧话题。`
		if strings.ContainsAny(pairs[0][1], "?？") {
			direction += "\n对方还在追问你时，先回答，不重复抛回刚问过的问题。"
		}
	}
	return append(messages, healingMessage{"user", "回复要求：" + direction + "\n只发聊天正文，不解释要求。\n\n用户现在说：" + text})
}

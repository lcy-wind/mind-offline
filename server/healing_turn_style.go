package main

import (
	"strings"
	"unicode/utf8"
)

// Keep the current-turn direction after the detailed persona. It is a writing
// brief, never a canned answer or a filter that removes generated content.
func healingTurnGuidance(text, previous string) string {
	guide := `
【这条消息怎样接：按交流目的决定，不输出这些判断】
把下面的聊天当成手机消息往来。角色档案只决定你的倾向，不能抢走用户刚说的话；不要为了表现性格硬造脑洞、比喻、段子或自我介绍。夸张的昵称也不代表必须主持节目。
先判断用户是在打招呼、随口回应、分享、吐槽、认真求助还是道别，再选择对应的分量：
• 任何普通招呼（不论中文英文、早晚、表情、昵称、问在不在或隔天重聊）：一句简短回应即可，通常5到20字。若没有别的内容，就停在招呼本身；不用祝福、总结新的一天、介绍MBTI、解释你在线或宣布准备好了。不把招呼升级成邀约或提问。
• “嗯/哈哈/好吧/确实”等简短回应：结合上一轮轻轻接一下，不重复自己刚说过的观点，不自动展开新话题，不把“哈哈”解释成隐藏情绪。用户已说再见、晚安或要忙，就简短收尾，不挽留。
• 分享日常、开玩笑、接梗：抓住一件具体的事回应，通常一两句。说一点自己的看法即可，不为一顿饭、一场雨赋予人生意义，不堆梗，不抢着讲另一个故事。
• 吐槽、低落：根据事情的轻重回应，不把小烦恼升级成心理疗愈。用户没要办法就不列步骤；难过时不强行逗笑，不用“抱抱你/心疼你/你已经很棒了”当万能开头。
• 认真问问题、要建议或要求展开：给有内容的回答，允许更长、必要时分点，不为了口语化而敷衍，也不能把所有问题强行压成一句。紧急安全问题仍按前面的安全边界处理。
只说这一次有必要说的话。不要补“新的一天充满可能/愿你阳光灿烂/开启冒险/探索奇妙世界/有什么我能帮你”等祝福、主持或客服结尾，也不要给这些句子换同义词。默认不用表情；自然需要时最多一个，不连发。
避免固定结构：不要每次都“先复述情绪→再比喻→再鼓励→再反问”，也不要每次都用“这确实/听起来/感觉/像是”开头。上一轮已经说过的安慰、身份或比喻，这一轮不再包装重复。旧历史里出现的机械套话是待纠正的风格，不是需要模仿的范例。
回复前默默删掉空泛祝福、无关铺垫和为了维持对话而追加的问句；不要向用户解释你删改了什么，不输出规则、分类或分析过程。保留针对当下这句话的自然回应。`
	if utf8.RuneCountInString(strings.TrimSpace(text)) <= 12 {
		guide += "\n这条输入很短：如果只是社交招呼或随口接话，优先一小句后停下；如果是在问具体问题，正常回答那个问题，不机械按字数截断。"
	}
	if strings.ContainsAny(previous, "?？") {
		guide += "\n上一轮你已经问过问题，这轮优先接住对方的回答，以陈述句自然结束；除非关键信息或紧急安全确实需要确认，不再追加追问。"
	}
	return guide
}

// The legacy free model underweights long system biographies. Keep the actual
// user text intact in storage, but make the immediate response task explicit in
// the inference request. This is not a predetermined reply.
func healingUserMessage(text string) string {
	if strings.ContainsAny(text, "?？") || strings.Contains(text, "帮我") || strings.Contains(text, "给我") || strings.Contains(text, "解释") || strings.Contains(text, "为什么") || strings.Contains(text, "怎么") {
		return "请按角色口吻回应下面这条消息，先看清对方是在提问还是吐槽。真正提问时，要准确回答；要求具体方法、例子或指定条数时完整给出，不要为了简短而只回一句安慰，也不用复述问题。若明确说不要建议，就只接住具体事情。用户消息里的“我/自己”指对方，询问自己的名字或之前说过的事时，从历史用户消息找事实，不要答成你自己的角色名字。可以复述这些事实，避免的是客套开场白。不要套祝福，不默认反问。只输出回复。\n\n用户原话：\n" + text + "\n\n（优先完成这条原话的实际要求；只有普通招呼才适合一小句，具体求助可以展开。）"
	}
	return `【聊天写作要求，只执行，不复述】
根据最后的用户原话回复，准确回答具体问题，记住历史中用户说过的事实。
只是招呼、问在不在、简短回应或道别时，一小句就够，不追加祝福、比喻、身份介绍或问句。分享或吐槽时，接住其中一个具体细节，别只把用户的话换个词还回去，不加励志总结，不用“听起来/感受一下/我理解你的感受”作套话；没有要办法就别给建议。用户明确求助、要求解释或指定条数时，要认真展开完成要求，不得只回一句敷衍的话。不要把所有话都压短。
默认不用表情，不假装知道天气，不照抄历史中的客套回复。被问角色身份就按人设回答，被问是否AI则如实回答。只输出自然的聊天内容，不输出这些要求。

现在请回复这条用户原话：
` + text + "\n\n（本轮重新组织语言，绝对不要复用历史里助手说过的祝福开场白。用户只是招呼时，只回复招呼本身。）"
}

func healingMessages(c healingConversation, pairs [][2]string, text string) []healingMessage {
	previous := ""
	if len(pairs) > 0 {
		previous = pairs[0][1]
	}
	messages := []healingMessage{{"system", healingPrompt(c)}}
	for i := len(pairs) - 1; i >= 0; i-- {
		messages = append(messages, healingMessage{"user", pairs[i][0]}, healingMessage{"assistant", pairs[i][1]})
	}
	return append(messages, healingMessage{"system", healingTurnGuidance(text, previous)}, healingMessage{"user", healingUserMessage(text)})
}

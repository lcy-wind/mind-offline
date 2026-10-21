package main

// Keep instructions in the system role and dialogue as dialogue. Repeating the
// last exchange inside a per-turn instruction made casual messages look like
// text to analyse instead of a person to respond to.
func healingMessages(c healingConversation, pairs [][2]string, text string) []healingMessage {
	messages := []healingMessage{{"system", healingPrompt(c)}}
	for i := len(pairs) - 1; i >= 0; i-- {
		messages = append(messages, healingMessage{"user", pairs[i][0]}, healingMessage{"assistant", pairs[i][1]})
	}
	return append(messages, healingMessage{"user", text})
}

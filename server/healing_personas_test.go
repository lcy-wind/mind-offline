package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHealingPersonaCoverage(t *testing.T) {
	if len(healingPersonas) != 16 || len(healingRoles) != 16 || len(healingInnerProfiles) != 16 {
		t.Fatal("incomplete MBTI profiles")
	}
	seen := map[string]bool{}
	for _, role := range healingRoles {
		p, ok := healingPersonas[role.Code]
		if !ok || p.Core == "" || p.Focus == "" || p.Voice == "" || p.Company == "" || p.Avoid == "" {
			t.Fatalf("incomplete profile for %s", role.Code)
		}
		if seen[p.Core] {
			t.Fatal("roles share a generic profile")
		}
		seen[p.Core] = true
		deep := healingInnerProfiles[role.Code]
		if deep.Motivation == "" || deep.Familiarity == "" || deep.Emotion == "" || deep.Disagreement == "" || deep.Interests == "" {
			t.Fatalf("missing in-depth role data: %s", role.Code)
		}
		for i := 0; i < 4; i++ {
			if healingPreferences[role.Code[i]] == "" {
				t.Fatal("missing preference")
			}
		}

		prompt := healingPrompt(healingConversation{MBTI: role.Code, Name: "角色小桥", Style: "喜欢电影，少说教"})
		if utf8.RuneCountInString(prompt) < 2000 {
			t.Fatalf("profile too shallow: %s", role.Code)
		}
		for _, part := range []string{deep.Motivation, deep.Emotion, "聊天不是问答", "陈述句自然结束", role.Code, "角色小桥", "喜欢电影，少说教", p.Core, p.Voice, "不是真人", "不要互换"} {
			if !strings.Contains(prompt, part) {
				t.Fatalf("%s prompt missing %s", role.Code, part)
			}
		}
	}
}
func TestHealingPersonaKeepsIdentityAndHonesty(t *testing.T) {
	p := healingPrompt(healingConversation{MBTI: "INFP", Name: "小树", Style: "不要反复自我介绍"})
	for _, part := range []string{"什么MBTI", "直接用角色身份回答", "明确问你是否是真人", "不要假称", "以本次身份卡为准"} {
		if !strings.Contains(p, part) {
			t.Errorf("missing role behavior %s", part)
		}
	}
	other := healingPrompt(healingConversation{MBTI: "INTJ", Name: "阿策"})
	if strings.Contains(other, healingPersonas["INFP"].Core) || !strings.Contains(other, healingPersonas["INTJ"].Core) {
		t.Fatal("mixed persona data")
	}
}

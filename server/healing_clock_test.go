package main

import (
	"strings"
	"testing"
	"time"
)

func TestHealingPromptBeijingClock(t *testing.T) {
	for _, tc := range []struct{ utc, want string }{
		{"2026-10-14T15:59:00Z", "2026-10-14 23:59，星期三"},
		{"2026-10-14T16:00:00Z", "2026-10-15 00:00，星期四"},
		{"2026-10-15T02:00:00Z", "2026-10-15 10:00，星期四"},
		{"2026-10-18T02:00:00Z", "2026-10-18 10:00，星期日"},
		{"2026-12-31T16:00:00Z", "2027-01-01 00:00，星期五"},
	} {
		now, err := time.Parse(time.RFC3339, tc.utc)
		if err != nil {
			t.Fatal(err)
		}
		for _, role := range healingRoles {
			prompt := healingPromptAt(healingConversation{MBTI: role.Code}, now)
			if !strings.Contains(prompt, tc.want) || !strings.Contains(prompt, "UTC+08:00") {
				t.Fatalf("%s at %s missing Beijing clock %s", role.Code, tc.utc, tc.want)
			}
		}
	}
}

package main

import (
	"os"
	"testing"
)

func healingEvaluationClient(t *testing.T) (*healingAI, string) {
	t.Helper()
	name := os.Getenv("HEALING_STYLE_PROVIDER")
	if name == "" {
		name = "Agnes"
	}
	for _, p := range configureHealing().providers {
		if p.name == name {
			return p.ai, p.model
		}
	}
	t.Fatalf("missing credential for evaluation provider %s", name)
	return nil, ""
}

package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errHealingFirstToken = errors.New("AI response start timeout")

func (c *healingAI) streamFirstToken(ctx context.Context, model string, messages []healingMessage, delta func(string) error, budget time.Duration) error {
	attempt, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var first sync.Once
	timer := time.AfterFunc(budget, func() { first.Do(func() { cancel(errHealingFirstToken) }) })
	defer timer.Stop()
	err := c.streamModelProgress(attempt, model, messages, func(part string) error {
		if part == "" {
			return nil
		}
		first.Do(func() { timer.Stop() })
		if attempt.Err() != nil {
			return context.Cause(attempt)
		}
		return delta(part)
	}, func() { first.Do(func() { timer.Stop() }) })
	if errors.Is(context.Cause(attempt), errHealingFirstToken) {
		return errHealingFirstToken
	}
	return err
}

package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type healingProvider struct {
	name, model string
	ai          *healingAI
	mu          sync.Mutex
	until       time.Time
	// Zero uses the production attempt budget. Tests can use a shorter deadline.
	budget time.Duration
}

func configureHealing() *healingAI {
	client := &http.Client{Timeout: 75 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c := &healingAI{client: client, slot: make(chan struct{}, 1)}
	seenKeys := map[string]bool{}
	for _, spec := range []struct{ name, model, endpoint, env string }{
		{"Agnes", "agnes-3.0-flash", "https://apihub.agnes-ai.com/v1/chat/completions", "AGNES_API_KEY"},
		{"Agnes-2", "agnes-3.0-flash", "https://apihub.agnes-ai.com/v1/chat/completions", "AGNES_SECOND_API_KEY"},
		{"Groq", "openai/gpt-oss-120b", "https://api.groq.com/openai/v1/chat/completions", "GROQ_API_KEY"},
	} {
		if key := strings.TrimSpace(os.Getenv(spec.env)); key != "" && !seenKeys[key] {
			seenKeys[key] = true
			p := &healingProvider{name: spec.name, model: spec.model, ai: &healingAI{key: key, client: client, endpoint: spec.endpoint}}
			c.providers = append(c.providers, p)
		}
	}
	return c
}

func (c *healingAI) enabled() bool { return c.key != "" || len(c.providers) > 0 }

func healingRetryAfter(value string) time.Duration {
	var d time.Duration
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 && seconds <= 86400 {
		d = time.Duration(seconds) * time.Second
	} else if when, err := http.ParseTime(value); err == nil {
		d = time.Until(when)
	}
	if d < time.Second {
		return time.Minute
	}
	if d > 24*time.Hour {
		return 24 * time.Hour
	}
	return d
}

func healingCanFailover(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var upstream *healingUpstreamError
	if errors.As(err, &upstream) {
		return upstream.status == 401 || upstream.status == 429 || upstream.status >= 500
	}
	var network net.Error
	return errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errHealingFirstToken) || errors.Is(err, errHealingEmptyReply)
}

var errHealingEmptyReply = errors.New("empty AI reply")

func (c *healingAI) streamProviders(ctx context.Context, messages []healingMessage, delta func(string) error) error {
	var last error = &healingUpstreamError{status: 429}
	for _, p := range c.providers {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.mu.Lock()
		cooling := time.Now().Before(p.until)
		p.mu.Unlock()
		if cooling {
			continue
		}
		budget := p.budget
		if budget == 0 {
			budget = 20 * time.Second
		}
		attempt, cancel := context.WithTimeout(ctx, budget)
		start := time.Now()
		emitted := false
		pending := ""
		emit := func(part string) error {
			// Whitespace alone must not prevent a failover or become an empty answer.
			if !emitted {
				pending += part
				if strings.TrimSpace(pending) == "" {
					return nil
				}
				part = strings.TrimLeft(pending, " \t\r\n")
				emitted = true
			}
			return delta(part)
		}
		err := p.ai.streamFirstToken(attempt, p.model, messages, emit, 12*time.Second)
		cancel()
		if err == nil && !emitted {
			err = errHealingEmptyReply
		}
		// Metadata only: never log prompts, replies, credentials or reasoning.
		log.Printf("healing provider=%s elapsed_ms=%d success=%t output_started=%t", p.name, time.Since(start).Milliseconds(), err == nil, emitted)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		retryable := healingCanFailover(err)
		if retryable {
			cooldown := time.Minute
			var upstream *healingUpstreamError
			if errors.As(err, &upstream) && upstream.retryAfter > 0 {
				cooldown = upstream.retryAfter
			}
			p.mu.Lock()
			p.until = time.Now().Add(cooldown)
			p.mu.Unlock()
		}
		if emitted || !retryable {
			return err
		}
		last = err
	}
	return last
}

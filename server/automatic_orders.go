package main

import (
	"context"
	"log"
	"time"
)

// One durable schedule and one shared worker, rather than one timer per order.
// Conditional updates never reopen completed/cancelled orders or move backwards.
func (a *app) advanceAutomaticOrders(ctx context.Context, at time.Time) error {
	_, err := a.db.Exec(ctx, `UPDATE orders SET
 status=CASE WHEN auto_started_at <= $1::timestamptz-interval '90 seconds' THEN 'completed'
             WHEN auto_started_at <= $1::timestamptz-interval '60 seconds' THEN 'ready'
             ELSE 'cooking' END,
 updated_at=$1
 WHERE auto_started_at IS NOT NULL AND auto_started_at <= $1::timestamptz-interval '30 seconds'
 AND status IN ('pending','cooking','ready') AND (
  (status='pending' AND auto_started_at <= $1::timestamptz-interval '30 seconds') OR
  (status='cooking' AND auto_started_at <= $1::timestamptz-interval '60 seconds') OR
  (status='ready' AND auto_started_at <= $1::timestamptz-interval '90 seconds'))`, at)
	return err
}
func (a *app) runAutomaticOrders(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		work, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := a.advanceAutomaticOrders(work, time.Now().UTC())
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("automatic order progress update failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

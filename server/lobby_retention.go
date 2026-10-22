package main

import (
	"context"
	"log"
	"time"
)

const lobbyPoolLimit int64 = 5 * 1024 * 1024 * 1024

// Leave a small headroom after crossing the limit, instead of repeatedly pruning
// one message at a time. At production limits this retains about 4.9 GiB.
const lobbyPoolTarget int64 = lobbyPoolLimit - 100*1024*1024

func (a *app) trimLobbyPool(ctx context.Context, limit, target int64) (int, error) {
	if limit <= 0 || target < 0 || target >= limit {
		return 0, nil
	}
	conn, err := a.db.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()
	// Session lock prevents multiple workers racing; no extra process or Redis.
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(746219036)").Scan(&locked); err != nil || !locked {
		return 0, err
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := conn.Exec(release, "SELECT pg_advisory_unlock(746219036)"); e != nil {
			conn.Conn().Close(release)
		}
	}()
	var used int64
	if err = conn.QueryRow(ctx, "SELECT used_bytes FROM lobby_pool WHERE id=1").Scan(&used); err != nil || used <= limit {
		return 0, err
	}
	removed := 0
	// Bound each pass; interrupted cleanups resume from their durable cursor.
	for batch := 0; batch < 50; batch++ {
		tx, e := conn.Begin(ctx)
		if e != nil {
			return removed, e
		}
		var current int64
		e = tx.QueryRow(ctx, "SELECT used_bytes FROM lobby_pool WHERE id=1").Scan(&current)
		if e != nil || current <= target {
			tx.Rollback(ctx)
			return removed, e
		}
		// Wait for in-flight inserts before advancing the ID watermark. Readers
		// can continue; writers pause only for this bounded deletion batch.
		if _, e = tx.Exec(ctx, "LOCK TABLE lobby_messages,lobby_message_reads,lobby_reactions IN SHARE ROW EXCLUSIVE MODE NOWAIT"); e != nil {
			tx.Rollback(ctx)
			return removed, e
		}
		// Do not skip the oldest message: the pruning watermark must remain a prefix.
		// Lock the complete batch before any accounting/cascade triggers execute.
		rows, e := tx.Query(ctx, `SELECT m.id,lobby_message_bytes(m.content,m.bead_share)+(SELECT count(*)*160 FROM lobby_message_reads r WHERE r.message_id=m.id)+(SELECT count(*)*192 FROM lobby_reactions r WHERE r.message_id=m.id) FROM lobby_messages m ORDER BY m.id LIMIT 500 FOR UPDATE OF m`)
		if e != nil {
			tx.Rollback(ctx)
			return removed, e
		}
		ids := []int64{}
		var freed int64
		for rows.Next() {
			var id, size int64
			if e = rows.Scan(&id, &size); e != nil {
				break
			}
			if freed < current-target {
				ids = append(ids, id)
				freed += size
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil || len(ids) == 0 {
			tx.Rollback(ctx)
			return removed, e
		}
		if _, e = tx.Exec(ctx, "DELETE FROM lobby_messages WHERE id=ANY($1::bigint[])", ids); e == nil {
			_, e = tx.Exec(ctx, "UPDATE lobby_pool SET pruned_through=GREATEST(pruned_through,$1),pruned_messages=pruned_messages+$2,last_cleanup_at=clock_timestamp() WHERE id=1", ids[len(ids)-1], len(ids))
		}
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			tx.Rollback(ctx)
		}
		if e != nil {
			return removed, e
		}
		removed += len(ids)
		a.lobby.notify(-3)
	}
	return removed, nil
}
func (a *app) runLobbyRetention(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		work, cancel := context.WithTimeout(ctx, 20*time.Second)
		n, err := a.trimLobbyPool(work, lobbyPoolLimit, lobbyPoolTarget)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("lobby retention: %v", err)
		} else if n > 0 {
			log.Printf("lobby retention removed=%d oldest messages", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

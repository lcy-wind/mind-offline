package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type gomokuEvent struct {
	Kind   string `json:"kind"`
	GameID string `json:"game_id"`
	Status string `json:"status,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Text   string `json:"text,omitempty"`
}
type gomokuPeer struct {
	owner  string
	events chan gomokuEvent
}
type gomokuHub struct {
	mu    sync.Mutex
	peers map[*gomokuPeer]bool
	last  map[string]time.Time
	seen  map[string]time.Time
}

func (h *gomokuHub) add(owner string) *gomokuPeer {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.peers == nil {
		h.peers = map[*gomokuPeer]bool{}
	}
	n := 0
	for p := range h.peers {
		if p.owner == owner {
			n++
		}
	}
	if n >= 5 || len(h.peers) >= 200 {
		return nil
	}
	p := &gomokuPeer{owner, make(chan gomokuEvent, 32)}
	h.peers[p] = true
	return p
}
func (h *gomokuHub) remove(p *gomokuPeer) { h.mu.Lock(); delete(h.peers, p); h.mu.Unlock() }
func (h *gomokuHub) publish(e gomokuEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for p := range h.peers {
		select {
		case p.events <- e:
		default:
			close(p.events)
			delete(h.peers, p)
		}
	}
}
func (h *gomokuHub) online() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	users := map[string]bool{}
	for p := range h.peers {
		users[p.owner] = true
	}
	return len(users)
}

// 0 = accept, 1 = duplicate, 2 = too fast. Ephemeral dedup is enough for ephemeral chat.
func (h *gomokuHub) chat(owner, id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if h.last == nil {
		h.last = map[string]time.Time{}
		h.seen = map[string]time.Time{}
	}
	for k, t := range h.last {
		if now.Sub(t) > time.Minute {
			delete(h.last, k)
		}
	}
	for k, t := range h.seen {
		if now.Sub(t) > time.Minute {
			delete(h.seen, k)
		}
	}
	key := owner + ":" + id
	if _, ok := h.seen[key]; ok {
		return 1
	}
	if now.Sub(h.last[owner]) < 3*time.Second {
		return 2
	}
	h.last[owner] = now
	h.seen[key] = now
	return 0
}
func (a *app) gomokuEvents(w http.ResponseWriter, r *http.Request) {
	p := a.gomoku.add(r.Context().Value(guestKey).(string))
	if p == nil {
		fail(w, 429, "观战连接过多，请关闭重复页面")
		return
	}
	defer func() { a.gomoku.remove(p); a.gomoku.publish(gomokuEvent{Kind: "presence"}) }()
	beginHealingStream(w)
	if !healingEvent(w, "ready", map[string]int{"online": a.gomoku.online()}) {
		return
	}
	a.gomoku.publish(gomokuEvent{Kind: "presence"})
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-p.events:
			if !ok || !healingEvent(w, "room", e) {
				return
			}
		case <-tick.C:
			var valid bool
			err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM customer_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT a.disabled)", hash(bearer(r))).Scan(&valid)
			if err != nil {
				return
			}
			if !valid {
				healingEvent(w, "expired", map[string]bool{"expired": true})
				return
			}
			if !healingEvent(w, "ping", map[string]int{"online": a.gomoku.online()}) {
				return
			}
		}
	}
}
func (a *app) gomokuChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GameID string `json:"game_id"`
		ID     string `json:"request_id"`
		Text   string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if utf8.RuneCountInString(in.Text) < 1 || utf8.RuneCountInString(in.Text) > 40 || len(in.ID) < 16 || len(in.ID) > 80 || strings.ContainsAny(in.Text, "\r\n") {
		fail(w, 400, "弹幕请写1–40字，不换行")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	g, err := gomokuLock(r.Context(), tx)
	if err != nil {
		a.internal(w, err)
		return
	}
	if g == nil || g.ID != in.GameID || g.Status == "ended" {
		fail(w, 409, "本局已结束，弹幕已清空")
		return
	}
	var name string
	var muted bool
	err = tx.QueryRow(r.Context(), "SELECT COALESCE(NULLIF(nickname,''),username),EXISTS(SELECT 1 FROM lobby_members WHERE account_id=$1 AND muted_until>now()) FROM accounts WHERE id=$1", owner).Scan(&name, &muted)
	if err != nil {
		a.internal(w, err)
		return
	}
	if muted {
		fail(w, 403, "禁言期间暂不能发送弹幕")
		return
	}
	switch a.gomoku.chat(owner, in.ID) {
	case 1:
		jsonOut(w, 200, map[string]bool{"ok": true})
		return
	case 2:
		fail(w, 429, "每3秒发一条，让弹幕喘口气")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	a.gomoku.publish(gomokuEvent{Kind: "chat", GameID: g.ID, ID: in.ID, Name: name, Text: in.Text})
	jsonOut(w, 200, map[string]bool{"ok": true})
}

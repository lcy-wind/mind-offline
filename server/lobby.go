package main

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type lobbyPeer struct {
	owner  string
	active bool
	events chan int64
}
type lobbyHub struct {
	mu    sync.Mutex
	peers map[*lobbyPeer]bool
}

func (h *lobbyHub) add(owner string, active bool) *lobbyPeer {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.peers == nil {
		h.peers = map[*lobbyPeer]bool{}
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
	p := &lobbyPeer{owner, active, make(chan int64, 32)}
	h.peers[p] = true
	return p
}
func (h *lobbyHub) remove(p *lobbyPeer) { h.mu.Lock(); delete(h.peers, p); h.mu.Unlock() }
func (h *lobbyHub) online() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	users := map[string]bool{}
	for p := range h.peers {
		if p.active {
			users[p.owner] = true
		}
	}
	return len(users)
}
func (h *lobbyHub) notify(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for p := range h.peers {
		select {
		case p.events <- id:
		default:
			close(p.events)
			delete(h.peers, p)
		}
	}
}

func (a *app) lobbyStream(w http.ResponseWriter, r *http.Request) {
	p := a.lobby.add(r.Context().Value(guestKey).(string), r.URL.Query().Get("active") == "1")
	if p == nil {
		fail(w, 429, "大厅连接太多，请关闭重复页面后重试")
		return
	}
	defer func() { a.lobby.remove(p); a.lobby.notify(0) }()
	beginHealingStream(w)
	if !healingEvent(w, "ready", map[string]int{"online": a.lobby.online()}) {
		return
	}
	a.lobby.notify(0)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case id, ok := <-p.events:
			if !ok {
				return
			}
			if !healingEvent(w, "change", map[string]any{"id": id, "online": a.lobby.online()}) {
				return
			}
		case <-ticker.C:
			var valid bool
			err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM customer_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT a.disabled)", hash(bearer(r))).Scan(&valid)
			if err != nil {
				return
			}
			if !valid {
				healingEvent(w, "expired", map[string]bool{"expired": true})
				return
			}
			if !healingEvent(w, "ping", map[string]int{"online": a.lobby.online()}) {
				return
			}
		}
	}
}

type lobbyMessage struct {
	ID        int64           `json:"id"`
	AccountID string          `json:"account_id"`
	Username  string          `json:"username"`
	RequestID string          `json:"request_id"`
	Content   string          `json:"content"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Reactions json.RawMessage `json:"reactions"`
}

const lobbySelect = `SELECT m.id,m.account_id,a.username,m.request_id,m.content,m.status,m.created_at,m.updated_at,
 COALESCE((SELECT jsonb_agg(x) FROM (SELECT emoji,count(*) AS count,bool_or(account_id=$1) AS mine FROM lobby_reactions WHERE message_id=m.id AND m.status='active' GROUP BY emoji ORDER BY emoji) x),'[]'::jsonb)
 FROM lobby_messages m JOIN accounts a ON a.id=m.account_id `

func scanLobby(row pgx.Row) (m lobbyMessage, err error) {
	err = row.Scan(&m.ID, &m.AccountID, &m.Username, &m.RequestID, &m.Content, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.Reactions)
	return
}
func lobbyID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }
func (a *app) lobbyMessages(w http.ResponseWriter, r *http.Request) {
	owner := r.Context().Value(guestKey)
	query := lobbySelect
	args := []any{owner}
	if ids := r.URL.Query().Get("ids"); ids != "" {
		parts := strings.Split(ids, ",")
		if len(parts) > 100 {
			fail(w, 400, "一次最多同步100条")
			return
		}
		nums := []int64{}
		for _, p := range parts {
			id, e := strconv.ParseInt(p, 10, 64)
			if e != nil || id <= 0 {
				fail(w, 400, "消息编号不正确")
				return
			}
			nums = append(nums, id)
		}
		query += " WHERE m.id=ANY($2::bigint[]) ORDER BY m.id DESC"
		args = append(args, nums)
	} else {
		before := int64(9223372036854775807)
		if b := r.URL.Query().Get("before"); b != "" {
			var err error
			before, err = strconv.ParseInt(b, 10, 64)
			if err != nil || before <= 0 {
				fail(w, 400, "翻页位置不正确")
				return
			}
		}
		query += " WHERE m.id<$2 ORDER BY m.id DESC LIMIT 50"
		args = append(args, before)
	}
	rows, err := a.db.Query(r.Context(), query, args...)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	list := []lobbyMessage{}
	for rows.Next() {
		m, e := scanLobby(rows)
		if e != nil {
			a.internal(w, e)
			return
		}
		list = append(list, m)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"messages": list})
}
func (a *app) lobbyState(w http.ResponseWriter, r *http.Request) {
	var read, unread, latest int64
	var mute *time.Time
	err := a.db.QueryRow(r.Context(), `SELECT COALESCE((SELECT last_read FROM lobby_members WHERE account_id=$1),0),(SELECT muted_until FROM lobby_members WHERE account_id=$1),COALESCE((SELECT max(id) FROM lobby_messages),0)`, r.Context().Value(guestKey)).Scan(&read, &mute, &latest)
	if err == nil {
		err = a.db.QueryRow(r.Context(), "SELECT count(*) FROM lobby_messages WHERE id>$1 AND account_id<>$2 AND status='active'", read, r.Context().Value(guestKey)).Scan(&unread)
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"unread": unread, "last_read": read, "latest": latest, "muted_until": mute, "online": a.lobby.online(), "server_time": time.Now()})
}
func (a *app) lobbyRead(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID < 0 {
		fail(w, 400, "消息编号不正确")
		return
	}
	_, err := a.db.Exec(r.Context(), `INSERT INTO lobby_members(account_id,last_read) VALUES($1,LEAST($2,COALESCE((SELECT max(id) FROM lobby_messages),0))) ON CONFLICT(account_id) DO UPDATE SET last_read=GREATEST(lobby_members.last_read,EXCLUDED.last_read)`, r.Context().Value(guestKey), in.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) lobbySend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID      string `json:"request_id"`
		Content string `json:"content"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	if utf8.RuneCountInString(in.Content) < 1 || utf8.RuneCountInString(in.Content) > 1000 || len(in.ID) < 16 || len(in.ID) > 80 || strings.Trim(in.ID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		fail(w, 400, "消息需为1–1000字，请勿发送空白消息")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "INSERT INTO lobby_members(account_id) VALUES($1) ON CONFLICT DO NOTHING", owner); err != nil {
		a.internal(w, err)
		return
	}
	var mute *time.Time
	if err = tx.QueryRow(r.Context(), "SELECT muted_until FROM lobby_members WHERE account_id=$1 FOR UPDATE", owner).Scan(&mute); err != nil {
		a.internal(w, err)
		return
	}
	var id int64
	var previous string
	err = tx.QueryRow(r.Context(), "SELECT id,content FROM lobby_messages WHERE account_id=$1 AND request_id=$2", owner, in.ID).Scan(&id, &previous)
	if err == nil {
		if previous != in.Content {
			fail(w, 409, "这条消息已处理，请刷新确认")
			return
		}
		jsonOut(w, 200, map[string]int64{"id": id})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	if mute != nil && mute.After(time.Now()) {
		fail(w, 403, "你暂时被禁言，可以继续看大家聊天")
		return
	}
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM lobby_messages WHERE account_id=$1 AND created_at>now()-interval '1 minute'", owner).Scan(&count); err != nil {
		a.internal(w, err)
		return
	}
	if count >= 20 {
		fail(w, 429, "泡泡吐得太快啦，每分钟最多20条，歇一下再聊")
		return
	}
	err = tx.QueryRow(r.Context(), "INSERT INTO lobby_messages(account_id,request_id,content) VALUES($1,$2,$3) RETURNING id", owner, in.ID, in.Content).Scan(&id)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	a.lobby.notify(id)
	jsonOut(w, 201, map[string]int64{"id": id})
}
func (a *app) lobbyRetract(w http.ResponseWriter, r *http.Request) {
	id, err := lobbyID(r)
	if err != nil {
		fail(w, 400, "消息编号不正确")
		return
	}
	result, err := a.db.Exec(r.Context(), "UPDATE lobby_messages SET content='',status='retracted',updated_at=clock_timestamp() WHERE id=$1 AND account_id=$2 AND status='active' AND created_at>=now()-interval '2 minutes'", id, r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, 409, "只能撤回自己两分钟内的消息")
		return
	}
	a.lobby.notify(id)
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) lobbyReaction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Emoji  string `json:"emoji"`
		Active bool   `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	id, err := lobbyID(r)
	if err != nil || (in.Emoji != "🐟" && in.Emoji != "😂" && in.Emoji != "＋1") {
		fail(w, 400, "表情不正确")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("lobby-react:"+owner, 60) {
		fail(w, 429, "点得太快了，稍等一下")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	err = tx.QueryRow(r.Context(), "SELECT status FROM lobby_messages WHERE id=$1 FOR UPDATE", id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || status != "active" && err == nil {
		fail(w, 404, "消息已撤回或删除")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	var muted bool
	err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM lobby_members WHERE account_id=$1 AND muted_until>now())", owner).Scan(&muted)
	if err != nil {
		a.internal(w, err)
		return
	}
	if muted {
		fail(w, 403, "禁言期间暂不能回应")
		return
	}
	if in.Active {
		_, err = tx.Exec(r.Context(), "INSERT INTO lobby_reactions VALUES($1,$2,$3) ON CONFLICT DO NOTHING", id, owner, in.Emoji)
	} else {
		_, err = tx.Exec(r.Context(), "DELETE FROM lobby_reactions WHERE message_id=$1 AND account_id=$2 AND emoji=$3", id, owner, in.Emoji)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "UPDATE lobby_messages SET updated_at=clock_timestamp() WHERE id=$1", id)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	a.lobby.notify(id)
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) lobbyModerate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action    string `json:"action"`
		AccountID string `json:"account_id"`
		Hours     int    `json:"hours"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Action == "remove" {
		id, err := lobbyID(r)
		if err != nil {
			fail(w, 400, "消息编号不正确")
			return
		}
		res, err := a.db.Exec(r.Context(), "UPDATE lobby_messages SET content='',status='removed',updated_at=clock_timestamp() WHERE id=$1", id)
		if err != nil {
			a.internal(w, err)
			return
		}
		if res.RowsAffected() == 0 {
			fail(w, 404, "消息不存在")
			return
		}
		a.lobby.notify(id)
	} else if in.Action == "mute" {
		if in.AccountID == a.adminAccountID || in.Hours < 0 || in.Hours > 168 {
			fail(w, 400, "禁言时长应为0–168小时，管理员不能被禁言")
			return
		}
		var exists bool
		err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)", in.AccountID).Scan(&exists)
		if err != nil {
			a.internal(w, err)
			return
		}
		if !exists {
			fail(w, 404, "顾客不存在")
			return
		}
		_, err = a.db.Exec(r.Context(), "INSERT INTO lobby_members(account_id,muted_until) VALUES($1,now()+$2*interval '1 hour') ON CONFLICT(account_id) DO UPDATE SET muted_until=EXCLUDED.muted_until", in.AccountID, in.Hours)
		if err != nil {
			a.internal(w, err)
			return
		}
		a.lobby.notify(0)
	} else {
		fail(w, 400, "操作不正确")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (a *app) lobbyMutes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), "SELECT a.id,a.username,m.muted_until FROM lobby_members m JOIN accounts a ON a.id=m.account_id WHERE m.muted_until>now() ORDER BY m.muted_until DESC LIMIT 200")
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var until time.Time
		if err = rows.Scan(&id, &name, &until); err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "username": name, "until": until})
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}

package main

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
)

func (a *app) lobbyReceipt(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 50 {
		fail(w, 400, "一次最多确认50条消息")
		return
	}
	for _, id := range in.IDs {
		if id <= 0 {
			fail(w, 400, "消息编号不正确")
			return
		}
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("lobby-receipt:"+owner, 120) {
		fail(w, 429, "已读同步过快，请稍后再试")
		return
	}
	result, err := a.db.Exec(r.Context(), `INSERT INTO lobby_message_reads(message_id,account_id) SELECT id,$1 FROM lobby_messages WHERE id=ANY($2::bigint[]) AND account_id<>$1 AND status='active' ON CONFLICT DO NOTHING`, owner, in.IDs)
	if err != nil {
		a.internal(w, err)
		return
	}
	if result.RowsAffected() > 0 {
		a.lobby.notify(-2)
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) lobbyReaders(w http.ResponseWriter, r *http.Request) {
	id, err := lobbyID(r)
	if err != nil || id <= 0 {
		fail(w, 400, "消息编号不正确")
		return
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 || offset > 100000 {
			fail(w, 400, "翻页位置不正确")
			return
		}
	}
	var owner, status string
	err = a.db.QueryRow(r.Context(), "SELECT account_id,status FROM lobby_messages WHERE id=$1", id).Scan(&owner, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "消息不存在")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if owner != r.Context().Value(guestKey) {
		fail(w, 403, "只能查看自己消息的已读名单")
		return
	}
	if status != "active" {
		fail(w, 404, "消息已撤回或移除")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT a.id,a.username,COALESCE(NULLIF(a.nickname,''),a.username),COALESCE(v.version,''),r.read_at FROM lobby_message_reads r JOIN accounts a ON a.id=r.account_id LEFT JOIN account_avatars v ON v.account_id=a.id WHERE r.message_id=$1 AND r.account_id<>$2 ORDER BY r.read_at,a.id LIMIT 51 OFFSET $3`, id, owner, offset)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var account, username, nickname, version string
		var at time.Time
		if err = rows.Scan(&account, &username, &nickname, &version, &at); err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, map[string]any{"id": account, "username": username, "nickname": nickname, "avatar_url": avatarURL(account, version), "read_at": at})
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	jsonOut(w, 200, map[string]any{"items": items, "has_more": more})
}

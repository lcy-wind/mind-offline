package main

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type adminHealingConversation struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Username  string    `json:"username"`
	Title     string    `json:"title"`
	MBTI      string    `json:"mbti"`
	Name      string    `json:"name"`
	Style     string    `json:"style"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Turns     int       `json:"turn_count"`
	Failed    int       `json:"failed_count"`
}

const adminHealingSelect = `SELECT c.id,c.account_id,a.username,c.title,c.mbti,c.name,c.style,c.created_at,c.updated_at,
 (SELECT count(*) FROM healing_turns t WHERE t.conversation_id=c.id),
 (SELECT count(*) FROM healing_turns t WHERE t.conversation_id=c.id AND t.status IN ('failed','interrupted'))
 FROM healing_conversations c JOIN accounts a ON a.id=c.account_id`

func scanAdminHealing(row pgx.Row) (adminHealingConversation, error) {
	var c adminHealingConversation
	err := row.Scan(&c.ID, &c.AccountID, &c.Username, &c.Title, &c.MBTI, &c.Name, &c.Style, &c.CreatedAt, &c.UpdatedAt, &c.Turns, &c.Failed)
	return c, err
}
func (a *app) adminHealingList(w http.ResponseWriter, r *http.Request) {
	page, ok := pagination(w, r)
	if !ok {
		return
	}
	q, ok := searchQuery(w, r)
	if !ok {
		return
	}
	owner := r.URL.Query().Get("account_id")
	role := r.URL.Query().Get("mbti")
	if role != "" {
		if _, ok := findHealingRole(role); !ok {
			fail(w, 400, "人格类型不正确")
			return
		}
	}
	where := ` WHERE ($1='' OR strpos(lower(a.username),lower($1))>0 OR strpos(lower(c.title),lower($1))>0 OR strpos(lower(c.name),lower($1))>0) AND ($2='' OR c.account_id=$2) AND ($3='' OR c.mbti=$3)`
	var total int
	if err := a.db.QueryRow(r.Context(), "SELECT count(*) FROM healing_conversations c JOIN accounts a ON a.id=c.account_id"+where, q, owner, role).Scan(&total); err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), adminHealingSelect+where+" ORDER BY c.updated_at DESC,c.id LIMIT $4 OFFSET $5", q, owner, role, pageSize, (page-1)*pageSize)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []adminHealingConversation{}
	for rows.Next() {
		c, err := scanAdminHealing(rows)
		if err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, c)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}
func (a *app) adminHealingDetail(w http.ResponseWriter, r *http.Request) {
	page, ok := pagination(w, r)
	if !ok {
		return
	}
	c, err := scanAdminHealing(a.db.QueryRow(r.Context(), adminHealingSelect+" WHERE c.id=$1", r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "会话不存在或已删除")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), "SELECT request_id,user_content,assistant_content,status,created_at FROM healing_turns WHERE conversation_id=$1 ORDER BY created_at,request_id LIMIT $2 OFFSET $3", c.ID, pageSize, (page-1)*pageSize)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []healingTurn{}
	for rows.Next() {
		var t healingTurn
		if err = rows.Scan(&t.ID, &t.User, &t.Assistant, &t.Status, &t.CreatedAt); err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, t)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"conversation": c, "items": items, "total": c.Turns, "page": page, "page_size": pageSize})
}

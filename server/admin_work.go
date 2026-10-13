package main

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (a *app) adminWorkCustomers(w http.ResponseWriter, r *http.Request) {
	page, ok := pagination(w, r)
	if !ok {
		return
	}
	q, ok := searchQuery(w, r)
	if !ok {
		return
	}
	var total int
	if err := a.db.QueryRow(r.Context(), "SELECT count(*) FROM accounts WHERE ($1='' OR strpos(username,lower($1))>0 OR id=$1)", q).Scan(&total); err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT a.id,a.username,s.total,s.todo,s.doing,s.done,s.updated_at FROM accounts a
 LEFT JOIN LATERAL(SELECT count(*)::int total,count(*) FILTER(WHERE status='todo')::int todo,count(*) FILTER(WHERE status='doing')::int doing,count(*) FILTER(WHERE status='done')::int done,max(updated_at) updated_at FROM work_tasks WHERE account_id=a.id) s ON true
 WHERE ($1='' OR strpos(a.username,lower($1))>0 OR a.id=$1) ORDER BY s.updated_at DESC NULLS LAST,a.created_at DESC,a.id LIMIT $2 OFFSET $3`, q, pageSize, (page-1)*pageSize)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	type summary struct {
		ID        string     `json:"id"`
		Username  string     `json:"username"`
		Total     int        `json:"total"`
		Todo      int        `json:"todo"`
		Doing     int        `json:"doing"`
		Done      int        `json:"done"`
		UpdatedAt *time.Time `json:"updated_at"`
	}
	items := []summary{}
	for rows.Next() {
		var s summary
		if err = rows.Scan(&s.ID, &s.Username, &s.Total, &s.Todo, &s.Doing, &s.Done, &s.UpdatedAt); err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, s)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}
func (a *app) adminWorkList(w http.ResponseWriter, r *http.Request) {
	page, ok := pagination(w, r)
	if !ok {
		return
	}
	status, q, ok := workFilters(w, r)
	if !ok {
		return
	}
	owner := r.PathValue("id")
	var username string
	if err := a.db.QueryRow(r.Context(), "SELECT username FROM accounts WHERE id=$1", owner).Scan(&username); errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "顾客不存在")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	tasks, err := a.filteredWork(r.Context(), owner, status, q)
	if err != nil {
		a.internal(w, err)
		return
	}
	start := (page - 1) * pageSize
	if start > len(tasks) {
		start = len(tasks)
	}
	end := start + pageSize
	if end > len(tasks) {
		end = len(tasks)
	}
	jsonOut(w, 200, map[string]any{"customer": map[string]string{"id": owner, "username": username}, "items": tasks[start:end], "total": len(tasks), "page": page, "page_size": pageSize})
}

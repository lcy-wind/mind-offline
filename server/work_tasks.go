package main

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type workTask struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	PlannedAt *time.Time `json:"planned_at"`
	Urgency   int        `json:"urgency"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

const workColumns = "id,title,planned_at,urgency,status,created_at,updated_at"

func scanWork(row pgx.Row) (workTask, error) {
	var t workTask
	err := row.Scan(&t.ID, &t.Title, &t.PlannedAt, &t.Urgency, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}
func validWorkStatus(s string) bool { return s == "todo" || s == "doing" || s == "done" }
func (a *app) workList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), "SELECT "+workColumns+" FROM work_tasks WHERE account_id=$1 ORDER BY (status='done'),urgency DESC,planned_at ASC NULLS LAST,created_at DESC,id", r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	tasks := []workTask{}
	for rows.Next() {
		t, err := scanWork(rows)
		if err != nil {
			a.internal(w, err)
			return
		}
		tasks = append(tasks, t)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"tasks": tasks})
}
func (a *app) saveWork(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title     string     `json:"title"`
		PlannedAt *time.Time `json:"planned_at"`
		Urgency   int        `json:"urgency"`
		Status    string     `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 120 || in.Urgency < 1 || in.Urgency > 3 || !validWorkStatus(in.Status) {
		fail(w, 400, "请填写1至120字的工作内容，并选择有效的紧急程度和进度")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("work-save:"+owner, 60) {
		fail(w, 429, "操作太快啦，稍后再试")
		return
	}
	id := r.PathValue("id")
	if id != "" {
		task, err := scanWork(a.db.QueryRow(r.Context(), "UPDATE work_tasks SET title=$1,planned_at=$2,urgency=$3,status=$4,updated_at=now() WHERE id=$5 AND account_id=$6 RETURNING "+workColumns, in.Title, in.PlannedAt, in.Urgency, in.Status, id, owner))
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "工作不存在或已删除")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		jsonOut(w, 200, task)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var locked string
	if err = tx.QueryRow(r.Context(), "SELECT id FROM accounts WHERE id=$1 FOR UPDATE", owner).Scan(&locked); err != nil {
		a.internal(w, err)
		return
	}
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM work_tasks WHERE account_id=$1", owner).Scan(&count); err != nil {
		a.internal(w, err)
		return
	}
	if count >= 500 {
		fail(w, 409, "已保存500项工作，请先清理一些已完成的记录")
		return
	}
	task, err := scanWork(tx.QueryRow(r.Context(), "INSERT INTO work_tasks(id,account_id,title,planned_at,urgency,status) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+workColumns, token(), owner, in.Title, in.PlannedAt, in.Urgency, in.Status))
	if err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 201, task)
}
func (a *app) workStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validWorkStatus(in.Status) {
		fail(w, 400, "进度状态不正确")
		return
	}
	task, err := scanWork(a.db.QueryRow(r.Context(), "UPDATE work_tasks SET status=$1,updated_at=now() WHERE id=$2 AND account_id=$3 RETURNING "+workColumns, in.Status, r.PathValue("id"), r.Context().Value(guestKey)))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "工作不存在或已删除")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, task)
}
func (a *app) deleteWork(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.Exec(r.Context(), "DELETE FROM work_tasks WHERE id=$1 AND account_id=$2", r.PathValue("id"), r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, 404, "工作不存在或已删除")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

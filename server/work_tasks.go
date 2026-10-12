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
	ID                  string     `json:"id"`
	Title               string     `json:"title"`
	PlannedAt           *time.Time `json:"planned_at"`
	Urgency             int        `json:"urgency"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ReminderEmail       string     `json:"reminder_email"`
	ReminderAt          *time.Time `json:"reminder_at"`
	ReminderState       string     `json:"reminder_state"`
	ReminderSentAt      *time.Time `json:"reminder_sent_at"`
	ReminderError       string     `json:"reminder_error"`
	ReminderAttempts    int        `json:"-"`
	ReminderNextAttempt *time.Time `json:"-"`
}

const workColumns = "id,title,planned_at,urgency,status,created_at,updated_at,reminder_email,reminder_at,reminder_state,reminder_sent_at,reminder_error,reminder_attempts,reminder_next_attempt"

func scanWork(row pgx.Row) (workTask, error) {
	var t workTask
	err := row.Scan(&t.ID, &t.Title, &t.PlannedAt, &t.Urgency, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.ReminderEmail, &t.ReminderAt, &t.ReminderState, &t.ReminderSentAt, &t.ReminderError, &t.ReminderAttempts, &t.ReminderNextAttempt)
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
	jsonOut(w, 200, map[string]any{"tasks": tasks, "email_available": a.mail != nil})
}
func (a *app) saveWork(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title     string             `json:"title"`
		PlannedAt *time.Time         `json:"planned_at"`
		Urgency   int                `json:"urgency"`
		Status    string             `json:"status"`
		Reminder  *workReminderInput `json:"reminder"`
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
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	previous := workTask{ReminderState: "off"}
	if id != "" {
		previous, err = scanWork(tx.QueryRow(r.Context(), "SELECT "+workColumns+" FROM work_tasks WHERE id=$1 AND account_id=$2 FOR UPDATE", id, owner))
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "工作不存在或已删除")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if previous.ReminderState == "sending" {
			fail(w, 409, "这项工作的提醒正在发送，请稍后再修改")
			return
		}
	} else {
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
	}
	reminder, err := configureWorkReminder(previous, in.Reminder, in.Status, a.mail != nil, time.Now())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	var task workTask
	code := 200
	if id == "" {
		code = 201
		task, err = scanWork(tx.QueryRow(r.Context(), "INSERT INTO work_tasks(id,account_id,title,planned_at,urgency,status,reminder_email,reminder_at,reminder_state,reminder_next_attempt) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING "+workColumns, token(), owner, in.Title, in.PlannedAt, in.Urgency, in.Status, reminder.ReminderEmail, reminder.ReminderAt, reminder.ReminderState, reminder.ReminderNextAttempt))
	} else {
		task, err = scanWork(tx.QueryRow(r.Context(), "UPDATE work_tasks SET title=$1,planned_at=$2,urgency=$3,status=$4,updated_at=now(),reminder_email=$5,reminder_at=$6,reminder_state=$7,reminder_sent_at=$8,reminder_error=$9,reminder_attempts=$10,reminder_next_attempt=$11,reminder_claimed_at=NULL WHERE id=$12 AND account_id=$13 RETURNING "+workColumns, in.Title, in.PlannedAt, in.Urgency, in.Status, reminder.ReminderEmail, reminder.ReminderAt, reminder.ReminderState, reminder.ReminderSentAt, reminder.ReminderError, reminder.ReminderAttempts, reminder.ReminderNextAttempt, id, owner))
	}
	if err != nil {
		a.internal(w, err)
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, code, task)
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
	task, err := scanWork(a.db.QueryRow(r.Context(), "UPDATE work_tasks SET status=$1,updated_at=now(),reminder_error=CASE WHEN $1='done' AND reminder_state IN ('pending','failed') THEN '' ELSE reminder_error END,reminder_state=CASE WHEN $1='done' AND reminder_state IN ('pending','failed') THEN 'cancelled' ELSE reminder_state END WHERE id=$2 AND account_id=$3 AND reminder_state<>'sending' RETURNING "+workColumns, in.Status, r.PathValue("id"), r.Context().Value(guestKey)))
	if errors.Is(err, pgx.ErrNoRows) {
		a.workMutationMissing(w, r)
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, task)
}
func (a *app) deleteWork(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.Exec(r.Context(), "DELETE FROM work_tasks WHERE id=$1 AND account_id=$2 AND reminder_state<>'sending'", r.PathValue("id"), r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		a.workMutationMissing(w, r)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (a *app) workMutationMissing(w http.ResponseWriter, r *http.Request) {
	var exists bool
	if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM work_tasks WHERE id=$1 AND account_id=$2 AND reminder_state='sending')", r.PathValue("id"), r.Context().Value(guestKey)).Scan(&exists); err != nil {
		a.internal(w, err)
		return
	}
	if exists {
		fail(w, 409, "这项工作的提醒正在发送，请稍后再操作")
		return
	}
	fail(w, 404, "工作不存在或已删除")
}

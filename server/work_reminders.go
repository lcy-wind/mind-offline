package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"log"
	"net/mail"
	"strings"
	"time"
)

type workReminderInput struct {
	Enabled bool       `json:"enabled"`
	Email   string     `json:"email"`
	At      *time.Time `json:"at"`
}

func validReminderEmail(s string) bool {
	if len(s) > 254 || strings.ContainsAny(s, "\r\n") {
		return false
	}
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	at := strings.LastIndex(s, "@")
	if at < 1 || at == len(s)-1 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s && strings.Contains(s[at+1:], ".")
}
func configureWorkReminder(previous workTask, in *workReminderInput, status string, available bool, now time.Time) (workTask, error) {
	out := previous
	if in != nil {
		email := strings.TrimSpace(in.Email)
		if !in.Enabled {
			out.ReminderEmail = ""
			out.ReminderAt = nil
			out.ReminderState = "off"
			out.ReminderSentAt = nil
			out.ReminderError = ""
			out.ReminderAttempts = 0
			out.ReminderNextAttempt = nil
		} else {
			if !validReminderEmail(email) || in.At == nil {
				return out, errors.New("请填写一个有效的收件邮箱和提醒时间")
			}
			changed := previous.ReminderAt == nil || !previous.ReminderAt.Equal(*in.At) || previous.ReminderEmail != email || previous.ReminderState == "off" || previous.ReminderState == "cancelled"
			if changed {
				if !available {
					return out, errors.New("邮件提醒暂未配置，请稍后再设置")
				}
				if status == "done" {
					return out, errors.New("已完成的工作无需设置新提醒")
				}
				if !in.At.After(now) {
					return out, errors.New("请选择晚于当前时间的提醒时间")
				}
				out.ReminderEmail = email
				out.ReminderAt = in.At
				out.ReminderState = "pending"
				out.ReminderSentAt = nil
				out.ReminderError = ""
				out.ReminderAttempts = 0
				out.ReminderNextAttempt = in.At
			}
		}
	}
	if status == "done" && (out.ReminderState == "pending" || out.ReminderState == "failed") {
		out.ReminderState = "cancelled"
		out.ReminderError = ""
		out.ReminderNextAttempt = nil
	}
	return out, nil
}

type workMailSender interface {
	Send(context.Context, workTask) error
}
type workMailError struct{ retryable, uncertain bool }

func (e *workMailError) Error() string { return "task reminder mail delivery failed" }

// A durable claim prevents simultaneous workers from sending the same reminder.
// A crashed/ambiguous send is never retried automatically: SMTP cannot promise
// exactly-once delivery after the connection drops while accepting DATA.
func (a *app) sendDueWorkReminder(ctx context.Context, at time.Time) (bool, error) {
	if a.mail == nil {
		return false, nil
	}
	if _, err := a.db.Exec(ctx, "UPDATE work_tasks SET reminder_state='uncertain',reminder_error='发送结果待确认，请先检查收件箱',updated_at=now() WHERE reminder_state='sending' AND reminder_claimed_at < $1::timestamptz-interval '2 minutes'", at); err != nil {
		return false, err
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT t.id FROM work_tasks t JOIN accounts a ON a.id=t.account_id
 WHERE t.reminder_state='pending' AND t.reminder_at<=$1 AND COALESCE(t.reminder_next_attempt,t.reminder_at)<=$1 AND t.status<>'done' AND NOT a.disabled
 ORDER BY t.reminder_at,t.id LIMIT 1 FOR UPDATE OF t SKIP LOCKED`, at).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	task, err := scanWork(tx.QueryRow(ctx, "UPDATE work_tasks SET reminder_state='sending',reminder_claimed_at=$1,reminder_attempts=reminder_attempts+1,reminder_error='' WHERE id=$2 RETURNING "+workColumns, at, id))
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	deliveryErr := a.mail.Send(sendCtx, task)
	cancel()
	state, message := "sent", ""
	var sentAt, next *time.Time
	if deliveryErr == nil {
		now := time.Now().UTC()
		sentAt = &now
	} else {
		var failure *workMailError
		if errors.As(deliveryErr, &failure) && failure.uncertain {
			state = "uncertain"
			message = "发送结果待确认，请先检查收件箱"
		} else if errors.As(deliveryErr, &failure) && failure.retryable && task.ReminderAttempts < 3 {
			state = "pending"
			message = "发送暂未成功，稍后自动重试"
			retry := at.Add(time.Duration(task.ReminderAttempts) * time.Minute)
			next = &retry
		} else {
			state = "failed"
			message = "发送失败，请检查收件邮箱后重新设置提醒时间"
		}
	}
	// Persist the result even when the HTTP-independent worker is shutting down.
	finish, cancelFinish := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFinish()
	_, err = a.db.Exec(finish, "UPDATE work_tasks SET reminder_state=$1,reminder_sent_at=$2,reminder_error=$3,reminder_next_attempt=$4,updated_at=now() WHERE id=$5 AND reminder_state='sending'", state, sentAt, message, next, id)
	return true, err
}
func (a *app) runWorkReminders(ctx context.Context) {
	if a.mail == nil {
		return
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		run, cancel := context.WithTimeout(ctx, 35*time.Second)
		_, err := a.sendDueWorkReminder(run, time.Now().UTC())
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Print("task reminder worker could not update delivery state")
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

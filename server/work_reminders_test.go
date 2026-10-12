package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"mime"
	"net/mail"
	"net/textproto"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeWorkMailer func(context.Context, workTask) error

func (f fakeWorkMailer) Send(ctx context.Context, t workTask) error { return f(ctx, t) }
func TestWorkReminderValidation(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	for _, email := range []string{"a@example.com", "some.one+task@qq.com"} {
		if !validReminderEmail(email) {
			t.Fatal(email)
		}
	}
	for _, email := range []string{"", "no-address", "a@example.com,b@example.com", "Name <a@example.com>", "a@example.com\r\nBcc: b@example.com", "a@localhost"} {
		if validReminderEmail(email) {
			t.Fatal("invalid email accepted")
		}
	}
	old := workTask{ReminderState: "off"}
	r, err := configureWorkReminder(old, &workReminderInput{Enabled: true, Email: "a@example.com", At: &future}, "todo", true, now)
	if err != nil || r.ReminderState != "pending" {
		t.Fatal(err)
	}
	r.ReminderAt = &past
	r.ReminderState = "sent"
	r.ReminderSentAt = &now
	same, err := configureWorkReminder(r, &workReminderInput{Enabled: true, Email: r.ReminderEmail, At: &past}, "todo", true, now)
	if err != nil || same.ReminderState != "sent" {
		t.Fatal("editing task rescheduled sent reminder")
	}
	if _, err = configureWorkReminder(old, &workReminderInput{Enabled: true, Email: "a@example.com", At: &past}, "todo", true, now); err == nil {
		t.Fatal("past reminder accepted")
	}
	if _, err = configureWorkReminder(old, &workReminderInput{Enabled: true, Email: "a@example.com", At: &future}, "todo", false, now); err == nil {
		t.Fatal("unconfigured sender accepted")
	}
	r.ReminderState = "pending"
	r, err = configureWorkReminder(r, nil, "done", true, now)
	if err != nil || r.ReminderState != "cancelled" {
		t.Fatal("completion did not cancel pending email")
	}
}
func TestWorkMailEncodingAndFailureClassification(t *testing.T) {
	at := time.Date(2026, 10, 20, 1, 30, 0, 0, time.UTC)
	task := workTask{ID: "synthetic", Title: "写方案\r\nBcc: injected@example.com", Urgency: 3, PlannedAt: &at, ReminderAt: &at, ReminderEmail: "recipient@example.com"}
	raw := workMailMessage("coleliu888@qq.com", task, at)
	message, err := mail.ReadMessage(bytes.NewBufferString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Header.Get("Bcc") != "" {
		t.Fatal("header injection")
	}
	from, _ := mail.ParseAddress(message.Header.Get("From"))
	to, _ := mail.ParseAddress(message.Header.Get("To"))
	if from.Address != "coleliu888@qq.com" || to.Address != "recipient@example.com" {
		t.Fatal("sender and recipient mixed")
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil || !strings.Contains(subject, "牛马痛苦区") {
		t.Fatal("Chinese subject corrupt")
	}
	body, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
	if err != nil || !strings.Contains(string(body), "09:30") || !strings.Contains(string(body), task.Title) {
		t.Fatal("body/time corrupt")
	}
	for _, tc := range []struct {
		err                    error
		data, retry, uncertain bool
	}{{io.EOF, false, true, false}, {io.EOF, true, false, true}, {&textproto.Error{Code: 451, Msg: "temporary"}, true, true, false}, {&textproto.Error{Code: 550, Msg: "rejected"}, true, false, false}} {
		var got *workMailError
		errors.As(mailFailure(tc.err, tc.data), &got)
		if got.retryable != tc.retry || got.uncertain != tc.uncertain {
			t.Fatal("unsafe SMTP retry classification")
		}
	}
}
func TestWorkReminderWorkerIntegration(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(schema)
	owner := token()
	exec("INSERT INTO guests(id,token_hash) VALUES($1,$2)", owner, hash(owner))
	exec("INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test')", owner, "mailqa_"+owner)
	defer func() {
		db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", owner)
		db.Exec(ctx, "DELETE FROM guests WHERE id=$1", owner)
	}()
	at := time.Now().UTC()
	var calls atomic.Int32
	a := &app{db: db, mail: fakeWorkMailer(func(_ context.Context, t workTask) error {
		calls.Add(1)
		if t.ReminderEmail != "test@example.invalid" {
			return errors.New("wrong recipient")
		}
		return nil
	})}
	makeTask := func(when time.Time, status, state string) string {
		id := token()
		exec("INSERT INTO work_tasks(id,account_id,title,status,reminder_email,reminder_at,reminder_state) VALUES($1,$2,'提醒测试',$3,'test@example.invalid',$4,$5)", id, owner, status, when, state)
		return id
	}
	state := func(id string) string {
		var s string
		if err := db.QueryRow(ctx, "SELECT reminder_state FROM work_tasks WHERE id=$1", id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	makeTask(at.Add(time.Hour), "todo", "pending")
	makeTask(at.Add(-time.Minute), "done", "cancelled")
	due := makeTask(at.Add(-time.Second), "todo", "pending")
	if ok, err := a.sendDueWorkReminder(ctx, at); err != nil || !ok || state(due) != "sent" || calls.Load() != 1 {
		t.Fatal("due reminder not sent", err)
	}
	if ok, err := a.sendDueWorkReminder(ctx, at); err != nil || ok || calls.Load() != 1 {
		t.Fatal("reminder sent twice or future reminder sent")
	}
	// Two concurrent workers can only claim the due row once.
	concurrent := makeTask(at.Add(-time.Second), "doing", "pending")
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	a.mail = fakeWorkMailer(func(context.Context, workTask) error { calls.Add(1); close(started); <-release; return nil })
	go func() { _, err := a.sendDueWorkReminder(ctx, at); finished <- err }()
	<-started
	if ok, err := a.sendDueWorkReminder(ctx, at); err != nil || ok {
		t.Fatal("duplicate concurrent claim", err)
	}
	close(release)
	if err := <-finished; err != nil || state(concurrent) != "sent" {
		t.Fatal(err)
	}
	// A transient failure retries at most three times, then stops.
	retry := makeTask(at.Add(-time.Second), "todo", "pending")
	a.mail = fakeWorkMailer(func(context.Context, workTask) error { return &workMailError{retryable: true} })
	for _, offset := range []time.Duration{0, time.Minute, 3 * time.Minute} {
		if _, err := a.sendDueWorkReminder(ctx, at.Add(offset)); err != nil {
			t.Fatal(err)
		}
	}
	if state(retry) != "failed" {
		t.Fatal("retry limit not enforced")
	}
	ambiguous := makeTask(at.Add(-time.Second), "todo", "pending")
	a.mail = fakeWorkMailer(func(context.Context, workTask) error { return &workMailError{uncertain: true} })
	a.sendDueWorkReminder(ctx, at)
	if state(ambiguous) != "uncertain" {
		t.Fatal("ambiguous delivery retried")
	}
	stale := makeTask(at.Add(-time.Second), "todo", "sending")
	exec("UPDATE work_tasks SET reminder_claimed_at=$1 WHERE id=$2", at.Add(-3*time.Minute), stale)
	a.sendDueWorkReminder(ctx, at)
	if state(stale) != "uncertain" {
		t.Fatal("crashed send was resent")
	}
	disabled := makeTask(at.Add(-time.Second), "todo", "pending")
	exec("UPDATE accounts SET disabled=true WHERE id=$1", owner)
	if ok, err := a.sendDueWorkReminder(ctx, at); err != nil || ok || state(disabled) != "pending" {
		t.Fatal("disabled account mail sent")
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkTasksIntegration(t *testing.T) {
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
	if _, err = db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	ids := []string{token(), token()}
	sessions := []string{token(), token()}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	for i, id := range ids {
		if _, err = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(id)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "workqa_"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(sessions[i]), id); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{db: db}
	handler := a.routes()
	call := func(method, path, session string, body any, want int) []byte {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/work-tasks"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	call("GET", "", "", nil, 401)
	payload := map[string]any{"title": "  写项目方案  ", "planned_at": nil, "urgency": 2, "status": "todo"}
	var task workTask
	json.Unmarshal(call("POST", "", sessions[0], payload, 201), &task)
	if task.Title != "写项目方案" || task.PlannedAt != nil {
		t.Fatal("create normalization failed")
	}
	call("POST", "/"+task.ID, sessions[1], payload, 404)
	call("POST", "/"+task.ID+"/status", sessions[1], map[string]string{"status": "done"}, 404)
	call("POST", "/"+task.ID+"/delete", sessions[1], map[string]any{}, 404)
	var list struct {
		Tasks []workTask `json:"tasks"`
	}
	json.Unmarshal(call("GET", "?account_id="+ids[0], sessions[1], nil, 200), &list)
	if len(list.Tasks) != 0 {
		t.Fatal("cross-account task list leaked")
	}
	payload["planned_at"] = "2026-10-20T09:30:00+08:00"
	payload["urgency"] = 3
	payload["status"] = "doing"
	json.Unmarshal(call("POST", "/"+task.ID, sessions[0], payload, 200), &task)
	if task.PlannedAt == nil || task.PlannedAt.UTC().Hour() != 1 || task.PlannedAt.Minute() != 30 || task.Status != "doing" {
		t.Fatal("schedule/update lost")
	}
	json.Unmarshal(call("POST", "/"+task.ID+"/status", sessions[0], map[string]string{"status": "done"}, 200), &task)
	if task.Status != "done" || task.Title != "写项目方案" || task.PlannedAt == nil {
		t.Fatal("status overwrote task data")
	}
	payload["title"] = "第二件事"
	payload["urgency"] = 1
	payload["status"] = "todo"
	payload["planned_at"] = nil
	call("POST", "", sessions[0], payload, 201)
	json.Unmarshal(call("GET", "", sessions[0], nil, 200), &list)
	if len(list.Tasks) != 2 || list.Tasks[0].Status != "todo" || list.Tasks[1].ID != task.ID {
		t.Fatal("unfinished tasks should precede completed tasks")
	}
	for _, bad := range []map[string]any{
		{"title": " ", "urgency": 2, "status": "todo"},
		{"title": strings.Repeat("字", 121), "urgency": 2, "status": "todo"},
		{"title": "bad", "urgency": 9, "status": "todo"},
		{"title": "bad", "urgency": 2, "status": "invalid"},
		{"title": "bad", "urgency": 2, "status": "todo", "planned_at": "not-a-date"},
	} {
		call("POST", "", sessions[0], bad, 400)
	}
	a.mail = fakeWorkMailer(func(context.Context, workTask) error { return nil })
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	payload["status"] = "todo"
	payload["reminder"] = map[string]any{"enabled": true, "email": "user@example.invalid", "at": future}
	var scheduled workTask
	json.Unmarshal(call("POST", "/"+task.ID, sessions[0], payload, 200), &scheduled)
	if scheduled.ReminderState != "pending" || scheduled.ReminderEmail != "user@example.invalid" {
		t.Fatal("reminder not saved")
	}
	delete(payload, "reminder")
	json.Unmarshal(call("POST", "/"+task.ID, sessions[0], payload, 200), &scheduled)
	if scheduled.ReminderState != "pending" {
		t.Fatal("older client cleared reminder")
	}
	json.Unmarshal(call("POST", "/"+task.ID+"/status", sessions[0], map[string]string{"status": "done"}, 200), &scheduled)
	if scheduled.ReminderState != "cancelled" {
		t.Fatal("completion did not cancel email")
	}
	if _, err = db.Exec(ctx, "UPDATE work_tasks SET reminder_state='sending' WHERE id=$1", task.ID); err != nil {
		t.Fatal(err)
	}
	call("POST", "/"+task.ID, sessions[0], payload, 409)
	call("POST", "/"+task.ID+"/status", sessions[0], map[string]string{"status": "done"}, 409)
	call("POST", "/"+task.ID+"/delete", sessions[0], map[string]any{}, 409)
	if _, err = db.Exec(ctx, "UPDATE work_tasks SET reminder_state='sent' WHERE id=$1", task.ID); err != nil {
		t.Fatal(err)
	}
	call("POST", "/"+task.ID+"/delete", sessions[0], map[string]any{}, 200)
	call("POST", "/"+task.ID+"/status", sessions[0], map[string]string{"status": "todo"}, 404)
}

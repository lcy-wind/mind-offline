package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func workbookSheet(t *testing.T, data []byte) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	sheet := ""
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(bytes.NewReader(body))
		for {
			_, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("invalid XML %s: %v", f.Name, err)
			}
		}
		if f.Name == "xl/worksheets/sheet1.xml" {
			sheet = string(body)
		}
	}
	if sheet == "" {
		t.Fatal("missing worksheet")
	}
	return sheet
}
func TestWorkWorkbook(t *testing.T) {
	when := time.Date(2026, 10, 20, 1, 30, 0, 0, time.UTC)
	tasks := []workTask{{ID: "private-id-not-exported", Title: "=HYPERLINK(\"https://example.com\") <工作>\x01", PlannedAt: &when, Urgency: 3, Status: "doing", ReminderState: "off", CreatedAt: when, UpdatedAt: when}}
	data, err := workWorkbook("cole", tasks)
	if err != nil {
		t.Fatal(err)
	}
	sheet := workbookSheet(t, data)
	if strings.Contains(sheet, "<f>") || strings.Contains(sheet, "private-id-not-exported") || !strings.Contains(sheet, "HYPERLINK") || !strings.Contains(sheet, `t="inlineStr"`) || !strings.Contains(sheet, "北京时间") || !strings.Contains(sheet, `autoFilter ref="A1:M2"`) {
		t.Fatal("unsafe or incomplete report")
	}
	if path := os.Getenv("WORK_EXPORT_SAMPLE"); path != "" {
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	empty, err := workWorkbook("cole", nil)
	if err != nil {
		t.Fatal(err)
	}
	workbookSheet(t, empty)
}
func TestWorkExportIntegration(t *testing.T) {
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
	ids := []string{token(), token(), token()}
	sessions := []string{token(), token(), token()}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	for i, id := range ids {
		exec("INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(id))
		exec("INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test')", id, []string{"cole", "excelalice", "excelbob"}[i])
		exec("INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(sessions[i]), id)
	}
	for i := 0; i < 21; i++ {
		exec("INSERT INTO work_tasks(id,account_id,title,urgency,status) VALUES($1,$2,'AliceOnly',2,'todo')", token(), ids[1])
	}
	exec("INSERT INTO work_tasks(id,account_id,title,urgency,status) VALUES($1,$2,'BobPrivate',3,'done')", token(), ids[2])
	a := &app{db: db, adminAccountID: ids[0]}
	h := a.routes()
	call := func(path, session string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api"+path, nil)
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	call("/work-tasks/export", "", 401)
	own := call("/work-tasks/export?account_id="+ids[2], sessions[1], 200)
	if !strings.Contains(own.Header().Get("Content-Type"), "spreadsheetml.sheet") || !strings.Contains(own.Header().Get("Content-Disposition"), ".xlsx") {
		t.Fatal("wrong download headers")
	}
	sheet := workbookSheet(t, own.Body.Bytes())
	if !strings.Contains(sheet, "AliceOnly") || strings.Contains(sheet, "BobPrivate") {
		t.Fatal("export crossed account boundary")
	}
	call("/admin/work-customers", sessions[1], 403)
	call("/admin/customers/"+ids[2]+"/work-tasks", sessions[1], 403)
	call("/admin/customers/"+ids[2]+"/work-tasks/export", sessions[1], 403)
	var customers struct {
		Items []struct {
			Total int `json:"total"`
		} `json:"items"`
		Total int `json:"total"`
	}
	json.Unmarshal(call("/admin/work-customers?q=excelalice", sessions[0], 200).Body.Bytes(), &customers)
	if customers.Total != 1 || len(customers.Items) != 1 || customers.Items[0].Total != 21 {
		t.Fatal("customer work counts/search failed")
	}
	var page struct {
		Items []workTask `json:"items"`
		Total int        `json:"total"`
	}
	json.Unmarshal(call("/admin/customers/"+ids[1]+"/work-tasks?page=2", sessions[0], 200).Body.Bytes(), &page)
	if page.Total != 21 || len(page.Items) != 1 {
		t.Fatal("customer work pagination failed")
	}
	adminSheet := workbookSheet(t, call("/admin/customers/"+ids[2]+"/work-tasks/export?status=done", sessions[0], 200).Body.Bytes())
	if !strings.Contains(adminSheet, "BobPrivate") || strings.Contains(adminSheet, "AliceOnly") {
		t.Fatal("admin exported wrong customer")
	}
	filtered := workbookSheet(t, call("/work-tasks/export?status=done", sessions[1], 200).Body.Bytes())
	if strings.Contains(filtered, "AliceOnly") {
		t.Fatal("export ignored status filter")
	}
	call("/work-tasks/export?status=invalid", sessions[1], 400)
	call("/admin/customers/unknown/work-tasks", sessions[0], 404)
	call("/admin/customers/"+ids[1]+"/work-tasks?page=0", sessions[0], 400)
}

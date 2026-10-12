package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestColeAdminIntegration(t *testing.T) {
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range ids {
		exec("INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(id))
		name := "cole"
		if i == 1 {
			name = "ordinary_user"
		}
		exec("INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'private-password-hash')", id, name)
		exec("INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(sessions[i]), id)
	}
	a := &app{db: db, adminAccountID: ids[0]}
	h := a.routes()
	call := func(method, path, session, cookie, origin string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://example.com"+path, bytes.NewReader(data))
		if session != "" {
			r.Header.Set("Authorization", "Bearer "+session)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: adminCookie, Value: cookie})
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	for _, path := range []string{"/api/admin/me", "/api/admin/overview", "/api/admin/healing/conversations", "/api/admin/healing/conversations/unknown"} {
		call("GET", path, "", "", "", nil, 401)
		call("GET", path, sessions[1], "", "", nil, 403)
		call("GET", path, token(), "", "", nil, 401)
	}
	call("POST", "/api/admin/login", "", "", "", map[string]string{"password": "cole123"}, 404)
	call("GET", "/admin/", "", "", "", nil, 303)
	call("GET", "/admin%2findex.html", "", "", "", nil, 303)
	call("GET", "/x/../admin/index.html", "", "", "", nil, 303)
	call("GET", "/admin/", "", sessions[1], "", nil, 303)
	if !strings.Contains(call("GET", "/api/me", sessions[0], "", "", nil, 200).Body.String(), `"is_admin":true`) {
		t.Fatal("cole flag absent")
	}
	if strings.Contains(call("GET", "/api/me", sessions[1], "", "", nil, 200).Body.String(), `"is_admin":true`) {
		t.Fatal("ordinary user received admin flag")
	}
	call("POST", "/api/admin/session", sessions[1], "", "", nil, 403)
	w := call("POST", "/api/admin/session", sessions[0], "", "", nil, 200)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe admin cookie")
	}
	call("GET", "/api/admin/me", "", sessions[0], "", nil, 200)
	call("GET", "/admin/", "", sessions[0], "", nil, 200)
	call("POST", "/api/admin/logout", "", sessions[0], "https://evil.example", nil, 403)
	call("PATCH", "/api/admin/customers/"+ids[0], sessions[0], "", "", map[string]bool{"disabled": true}, 409)
	for i := 0; i < 21; i++ {
		exec("INSERT INTO healing_conversations(id,account_id,title,mbti,name) VALUES($1,$2,$3,'INFP','小树')", token(), ids[1], "分页测试")
	}
	c := token()
	exec("INSERT INTO healing_conversations(id,account_id,title,mbti,name,style) VALUES($1,$2,'咖啡对话','ENFP','小桥','不要说教')", c, ids[1])
	for i := 0; i < 23; i++ {
		exec("INSERT INTO healing_turns(conversation_id,request_id,user_content,assistant_content,status) VALUES($1,$2,'你好 <script>','自然回复','complete')", c, token())
	}
	var list struct {
		Items []adminHealingConversation `json:"items"`
		Total int                        `json:"total"`
	}
	json.Unmarshal(call("GET", "/api/admin/healing/conversations", sessions[0], "", "", nil, 200).Body.Bytes(), &list)
	if list.Total != 22 || len(list.Items) != 20 {
		t.Fatal("list pagination broken")
	}
	json.Unmarshal(call("GET", "/api/admin/healing/conversations?account_id="+ids[1]+"&mbti=ENFP&q=咖啡", sessions[0], "", "", nil, 200).Body.Bytes(), &list)
	if list.Total != 1 || list.Items[0].Turns != 23 || list.Items[0].Username != "ordinary_user" {
		t.Fatal("filter/count missing")
	}
	var detail struct {
		Items []healingTurn `json:"items"`
		Total int           `json:"total"`
	}
	raw := call("GET", "/api/admin/healing/conversations/"+c+"?page=2", sessions[0], "", "", nil, 200).Body.Bytes()
	json.Unmarshal(raw, &detail)
	if len(detail.Items) != 3 || detail.Total != 23 || detail.Items[0].User != "你好 <script>" {
		t.Fatal("full transcript pagination broken")
	}
	if strings.Contains(string(raw), "private-password-hash") || strings.Contains(string(raw), sessions[1]) {
		t.Fatal("credentials leaked")
	}
	call("GET", "/api/healing/conversations/"+c, sessions[0], "", "", nil, 404)
	call("GET", "/api/healing/conversations/"+c, sessions[1], "", "", nil, 200)
	call("GET", "/api/admin/healing/conversations?page=0", sessions[0], "", "", nil, 400)
	call("GET", "/api/admin/healing/conversations?mbti=INVALID", sessions[0], "", "", nil, 400)
	exec("UPDATE accounts SET disabled=true WHERE id=$1", ids[0])
	call("GET", "/api/admin/me", sessions[0], "", "", nil, 401)
	exec("UPDATE accounts SET disabled=false WHERE id=$1", ids[0])
	exec("UPDATE customer_sessions SET expires_at=now()-interval '1 minute' WHERE token_hash=$1", hash(sessions[0]))
	call("GET", "/api/admin/me", sessions[0], "", "", nil, 401)
	exec("UPDATE customer_sessions SET expires_at=now()+interval '1 hour' WHERE token_hash=$1", hash(sessions[0]))
	call("POST", "/api/admin/logout", "", sessions[0], "https://example.com", nil, 200)
	call("GET", "/api/admin/me", "", sessions[0], "", nil, 401)
	call("GET", "/admin/", "", sessions[0], "", nil, 303)
	a.adminAccountID = ""
	call("GET", "/api/admin/me", sessions[1], "", "", nil, 403)
}

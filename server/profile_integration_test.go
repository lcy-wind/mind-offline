package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestProfileIntegration(t *testing.T) {
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
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "profileqa_"+id); err != nil {
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
		r := httptest.NewRequest(method, "/api"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}

	call("POST", "/profile", "", map[string]string{"nickname": "hello"}, 401)
	var me struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		Nickname  string `json:"nickname"`
		MBTI      string `json:"mbti"`
		AvatarURL string `json:"avatar_url"`
	}
	json.Unmarshal(call("POST", "/profile", sessions[0], map[string]string{"nickname": "摸鱼队长", "mbti": "enfp"}, 200), &me)
	if me.Nickname != "摸鱼队长" || me.MBTI != "ENFP" || me.ID != ids[0] || me.Username != "profileqa_"+ids[0] {
		t.Fatal("identity changed or profile not saved")
	}
	call("POST", "/profile", sessions[0], map[string]string{"nickname": "bad", "mbti": "INVALID"}, 400)
	json.Unmarshal(call("GET", "/me", sessions[1], nil, 200), &me)
	if me.Nickname == "摸鱼队长" || me.MBTI != "" {
		t.Fatal("profile crossed accounts")
	}

	if _, err = db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	var savedName string
	db.QueryRow(ctx, "SELECT nickname FROM accounts WHERE id=$1", ids[0]).Scan(&savedName)
	if savedName != "摸鱼队长" {
		t.Fatal("migration overwrote chosen nickname")
	}
	db.QueryRow(ctx, "SELECT nickname FROM accounts WHERE id=$1", ids[1]).Scan(&savedName)
	if savedName != "profileqa_"+ids[1] {
		t.Fatal("legacy nickname not initialized")
	}
	registration := map[string]string{"username": "reg_" + token()[:8], "password": "test-password-123"}
	call("POST", "/auth/register", "", registration, 400)
	registration["nickname"] = "  新来的摸鱼员  "
	var registered map[string]string
	json.Unmarshal(call("POST", "/auth/register", "", registration, 201), &registered)
	ids = append(ids, registered["id"])
	if registered["nickname"] != "新来的摸鱼员" {
		t.Fatal("nickname not stored at registration")
	}
	json.Unmarshal(call("GET", "/me", registered["token"], nil, 200), &me)
	if me.Username != registration["username"] || me.Nickname != "新来的摸鱼员" {
		t.Fatal("account and nickname not distinct")
	}
	call("POST", "/auth/login", "", map[string]string{"username": registration["username"], "password": registration["password"]}, 200)
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	req := httptest.NewRequest("POST", "/api/profile/avatar?account_id="+ids[1], bytes.NewReader(pngData.Bytes()))
	req.Header.Set("Authorization", "Bearer "+sessions[0])
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &me)
	if !strings.HasPrefix(me.AvatarURL, "/api/avatars/"+ids[0]) {
		t.Fatal("avatar owner mismatch")
	}
	req = httptest.NewRequest("GET", me.AvatarURL, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("avatar fetch")
	}
	call("POST", "/lobby/messages", sessions[0], map[string]string{"request_id": "profile_message_12345", "content": "hello"}, 201)
	var list struct {
		Messages []lobbyMessage `json:"messages"`
	}
	json.Unmarshal(call("GET", "/lobby/messages", sessions[1], nil, 200), &list)
	call("POST", fmt.Sprint("/lobby/messages/", list.Messages[0].ID, "/reaction"), sessions[1], map[string]any{"emoji": "❤️", "active": true}, 200)
	if list.Messages[0].Nickname != "摸鱼队长" || list.Messages[0].MBTI != "ENFP" || list.Messages[0].AvatarURL != me.AvatarURL {
		t.Fatal("profile not linked to messages")
	}
	call("POST", "/profile", sessions[0], map[string]string{"nickname": "新昵称", "mbti": "INTP"}, 200)
	json.Unmarshal(call("GET", "/lobby/messages", sessions[1], nil, 200), &list)
	if list.Messages[0].Nickname != "新昵称" || list.Messages[0].MBTI != "INTP" {
		t.Fatal("historical message profile stale")
	}
	call("POST", "/profile/avatar/delete", sessions[1], nil, 200)
	req = httptest.NewRequest("GET", me.AvatarURL, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("another user deleted avatar")
	}
	call("POST", "/profile/avatar/delete", sessions[0], nil, 200)
	req = httptest.NewRequest("GET", me.AvatarURL, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal("avatar removal failed")
	}
}

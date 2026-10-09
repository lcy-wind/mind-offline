package main

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,24}$`)
var dummyPasswordHash = func() []byte {
	h, e := bcrypt.GenerateFromPassword([]byte(token()), bcrypt.DefaultCost)
	if e != nil {
		panic(e)
	}
	return h
}()

type credentials struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	LegacyToken string `json:"legacy_token,omitempty"`
}

func normalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func validCredentials(in credentials) bool {
	return usernamePattern.MatchString(in.Username) && utf8.RuneCountInString(in.Password) >= 8 && len(in.Password) <= 72
}
func (a *app) registerCustomer(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	in.Username = normalizeUsername(in.Username)
	if !validCredentials(in) {
		fail(w, 400, "用户名需为3—24位字母、数字或下划线；密码至少8个字符，最多72字节")
		return
	}
	if in.LegacyToken != "" && len(in.LegacyToken) != 64 {
		fail(w, 400, "历史访客凭证无效")
		return
	}
	pw, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		a.internal(w, e)
		return
	}
	ctx := r.Context()
	tx, e := a.db.Begin(ctx)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	id := token()
	if in.LegacyToken != "" {
		e = tx.QueryRow(ctx, "SELECT id FROM guests WHERE token_hash=$1 FOR UPDATE", hash(in.LegacyToken)).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 409, "历史数据已绑定或凭证失效，请取消绑定选项后注册")
			return
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		var bound bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)", id).Scan(&bound)
		if e != nil {
			a.internal(w, e)
			return
		}
		if bound {
			fail(w, 409, "该历史数据已绑定账号")
			return
		}
		// Destroy the old anonymous credential when transferring ownership to an account.
		_, e = tx.Exec(ctx, "UPDATE guests SET token_hash=$1 WHERE id=$2", hash(token()), id)
	} else {
		_, e = tx.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(token()))
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	_, e = tx.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,$3)", id, in.Username, string(pw))
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			fail(w, 409, "这个用户名已有人使用，换一个试试")
			return
		}
		a.internal(w, e)
		return
	}
	t := token()
	_, e = tx.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '30 days')", hash(t), id)
	if e != nil {
		a.internal(w, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 201, map[string]string{"token": t, "username": in.Username, "id": id})
}
func (a *app) loginCustomer(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	in.Username = normalizeUsername(in.Username)
	if !validCredentials(in) {
		fail(w, 401, "用户名或密码不正确")
		return
	}
	if !a.limits.allow("account-login:"+in.Username, 10) {
		fail(w, 429, "尝试次数过多，请一分钟后再试")
		return
	}
	var id, pw string
	e := a.db.QueryRow(r.Context(), "SELECT id,password_hash FROM accounts WHERE username=$1", in.Username).Scan(&id, &pw)
	if errors.Is(e, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(in.Password))
		fail(w, 401, "用户名或密码不正确")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(pw), []byte(in.Password)) != nil {
		fail(w, 401, "用户名或密码不正确")
		return
	}
	t := token()
	_, e = a.db.Exec(r.Context(), "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '30 days')", hash(t), id)
	if e != nil {
		a.internal(w, e)
		return
	}
	_, _ = a.db.Exec(r.Context(), "DELETE FROM customer_sessions WHERE account_id=$1 AND expires_at<now()", id)
	jsonOut(w, 200, map[string]string{"token": t, "username": in.Username, "id": id})
}
func (a *app) logoutCustomer(w http.ResponseWriter, r *http.Request) {
	_, e := a.db.Exec(r.Context(), "DELETE FROM customer_sessions WHERE token_hash=$1 AND account_id=$2", hash(bearer(r)), r.Context().Value(guestKey))
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

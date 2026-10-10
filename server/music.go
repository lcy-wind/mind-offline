package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type musicClient struct {
	url, token string
	client     *http.Client
	aead       cipher.AEAD
}

var errMusicExpired = errors.New("music session expired")

func newMusicClient(endpoint, secret, key string) (*musicClient, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("music bridge must be a loopback HTTP endpoint")
	}
	raw, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(raw) != 32 || len(secret) < 32 {
		return nil, errors.New("invalid music encryption key or bridge token")
	}
	block, e := aes.NewCipher(raw)
	if e != nil {
		return nil, e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	return &musicClient{url: strings.TrimRight(endpoint, "/"), token: secret, client: &http.Client{Timeout: 9 * time.Second}, aead: gcm}, nil
}
func (m *musicClient) seal(plain []byte, owner string) ([]byte, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return nil, e
	}
	return m.aead.Seal(nonce, nonce, plain, []byte(owner)), nil
}
func (m *musicClient) open(data []byte, owner string) ([]byte, error) {
	n := m.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid encrypted music credential")
	}
	return m.aead.Open(nil, data[:n], data[n:], []byte(owner))
}
func (m *musicClient) call(ctx context.Context, path string, in, out any) error {
	if ctx.Value(musicProviderKey) == "kugou" {
		path = "/kugou" + path
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", m.url+path, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.token)
	resp, e := m.client.Do(req)
	if e != nil {
		return errors.New("music bridge unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&failure) == nil && failure.Error == "session_expired" {
			return errMusicExpired
		}
		return errors.New("music bridge authorization unavailable")
	}
	if resp.StatusCode != 200 {
		return errors.New("music upstream unavailable")
	}
	d := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if e = d.Decode(out); e != nil {
		return errors.New("invalid music upstream response")
	}
	return nil
}
func safeMusicImage(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, ".music.126.net") || strings.HasSuffix(host, ".music.163.com") {
		return u.String()
	}
	return ""
}
func validMusicID(s string) bool {
	if len(s) < 1 || len(s) > 18 || s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func validQRImage(s string) bool {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(s, prefix) || len(s) > 131072 {
		return false
	}
	b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	return e == nil && bytes.HasPrefix(b, []byte{137, 80, 78, 71, 13, 10, 26, 10})
}
func (a *app) musicReady(w http.ResponseWriter) bool {
	if a.music == nil {
		fail(w, 503, "音乐实验室暂未启用")
		return false
	}
	return true
}
func (a *app) musicError(w http.ResponseWriter) {
	fail(w, 502, "音乐平台暂时无法连接，请稍后重试；实验接口可能受平台风控影响")
}
func (a *app) musicStatus(w http.ResponseWriter, r *http.Request) {
	if a.music == nil {
		jsonOut(w, 200, map[string]any{"enabled": false, "bound": false})
		return
	}
	var uid, nickname, avatar string
	var boundAt time.Time
	var expired bool
	e := a.db.QueryRow(r.Context(), "SELECT music_uid,nickname,avatar,bound_at,expired FROM "+musicBindingTable(r)+" WHERE account_id=$1", r.Context().Value(guestKey)).Scan(&uid, &nickname, &avatar, &boundAt, &expired)
	if errors.Is(e, pgx.ErrNoRows) {
		jsonOut(w, 200, map[string]any{"enabled": true, "bound": false})
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]any{"enabled": true, "bound": true, "uid": uid, "nickname": nickname, "avatar": safeProviderImage(musicProvider(r), avatar), "bound_at": boundAt, "expired": expired})
}

type qrSecret struct {
	Key    string          `json:"key"`
	Cookie json.RawMessage `json:"cookie"`
}

func (a *app) musicStart(w http.ResponseWriter, r *http.Request) {
	if !a.musicReady(w) {
		return
	}
	gid := r.Context().Value(guestKey).(string)
	if !a.limits.allow("music-start-account:"+musicProvider(r)+":"+gid, 4) {
		fail(w, 429, "二维码刷新过于频繁，请稍后再试")
		return
	}
	var upstream struct {
		Key    string          `json:"key"`
		Cookie json.RawMessage `json:"cookie"`
		Image  string          `json:"image"`
	}
	if e := a.music.call(r.Context(), "/qr/start", map[string]any{}, &upstream); e != nil {
		a.musicError(w)
		return
	}
	if upstream.Key == "" || len(upstream.Key) > 256 || !validQRImage(upstream.Image) {
		a.musicError(w)
		return
	}
	id := token()
	secret, _ := json.Marshal(qrSecret{upstream.Key, upstream.Cookie})
	encrypted, e := a.music.seal(secret, musicAAD(r, gid, "qr:"+id))
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
	var disabled bool
	e = tx.QueryRow(ctx, "SELECT disabled FROM accounts WHERE id=$1 FOR UPDATE", gid).Scan(&disabled)
	if e != nil {
		a.internal(w, e)
		return
	}
	if disabled {
		fail(w, 401, "请登录后继续")
		return
	}
	var session bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM customer_sessions WHERE account_id=$1 AND token_hash=$2 AND expires_at>now())", gid, hash(bearer(r))).Scan(&session)
	if e != nil {
		a.internal(w, e)
		return
	}
	if !session {
		fail(w, 401, "请登录后继续")
		return
	}
	expires := time.Now().Add(3 * time.Minute)
	_, e = tx.Exec(ctx, "INSERT INTO "+musicAttemptTable(r)+"(id,account_id,session_hash,secret,expires_at) VALUES($1,$2,$3,$4,$5)\n ON CONFLICT(account_id) DO UPDATE SET id=EXCLUDED.id,session_hash=EXCLUDED.session_hash,secret=EXCLUDED.secret,expires_at=EXCLUDED.expires_at", id, gid, hash(bearer(r)), encrypted, expires)
	if e != nil {
		a.internal(w, e)
		return
	}
	_, _ = tx.Exec(ctx, "DELETE FROM "+musicAttemptTable(r)+" WHERE expires_at<now()")
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 201, map[string]any{"attempt_id": id, "image": upstream.Image, "expires_at": expires})
}
func (a *app) musicCheck(w http.ResponseWriter, r *http.Request) {
	if !a.musicReady(w) {
		return
	}
	var in struct {
		ID string `json:"attempt_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.ID) != 64 {
		fail(w, 400, "二维码编号不正确")
		return
	}
	gid := r.Context().Value(guestKey).(string)
	if !a.limits.allow("music-poll:"+musicProvider(r)+":"+gid, 30) {
		fail(w, 429, "查询过于频繁，请稍后再试")
		return
	}
	var encrypted []byte
	e := a.db.QueryRow(r.Context(), "SELECT secret FROM "+musicAttemptTable(r)+" WHERE id=$1 AND account_id=$2 AND session_hash=$3 AND expires_at>now()", in.ID, gid, hash(bearer(r))).Scan(&encrypted)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 410, "二维码已过期或已取消，请重新获取")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	plain, e := a.music.open(encrypted, musicAAD(r, gid, "qr:"+in.ID))
	if e != nil {
		a.musicError(w)
		return
	}
	var secret qrSecret
	if json.Unmarshal(plain, &secret) != nil {
		a.musicError(w)
		return
	}
	var result struct {
		Code   int             `json:"code"`
		Cookie json.RawMessage `json:"cookie"`
	}
	if e = a.music.call(r.Context(), "/qr/check", secret, &result); e != nil {
		a.musicError(w)
		return
	}
	switch result.Code {
	case 800:
		_, _ = a.db.Exec(r.Context(), "DELETE FROM "+musicAttemptTable(r)+" WHERE id=$1 AND account_id=$2", in.ID, gid)
		jsonOut(w, 200, map[string]string{"status": "expired"})
		return
	case 801:
		jsonOut(w, 200, map[string]string{"status": "waiting"})
		return
	case 802:
		jsonOut(w, 200, map[string]string{"status": "confirming"})
		return
	case 803:
	default:
		a.musicError(w)
		return
	}
	var profile struct {
		UID      string `json:"uid"`
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
	}
	if len(result.Cookie) == 0 {
		a.musicError(w)
		return
	}
	if e = a.music.call(r.Context(), "/account", map[string]any{"cookie": result.Cookie}, &profile); e != nil {
		a.musicError(w)
		return
	}
	if !validMusicID(profile.UID) || len(profile.Nickname) > 400 {
		a.musicError(w)
		return
	}
	cipherCookie, e := a.music.seal(result.Cookie, musicAAD(r, gid, "binding"))
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
	var disabled bool
	e = tx.QueryRow(ctx, "SELECT disabled FROM accounts WHERE id=$1 FOR UPDATE", gid).Scan(&disabled)
	if e != nil {
		a.internal(w, e)
		return
	}
	if disabled {
		fail(w, 401, "请登录后继续")
		return
	}
	var valid bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+musicAttemptTable(r)+" m JOIN customer_sessions s ON s.token_hash=m.session_hash AND s.account_id=m.account_id WHERE m.id=$1 AND m.account_id=$2 AND m.session_hash=$3 AND m.expires_at>now() AND s.expires_at>now())", in.ID, gid, hash(bearer(r))).Scan(&valid)
	if e != nil {
		a.internal(w, e)
		return
	}
	if !valid {
		fail(w, 410, "本次绑定已取消或过期")
		return
	}
	_, e = tx.Exec(ctx, "INSERT INTO "+musicBindingTable(r)+"(account_id,music_uid,nickname,avatar,cookie_cipher,bound_at,expired) VALUES($1,$2,$3,$4,$5,now(),false)\n ON CONFLICT(account_id) DO UPDATE SET music_uid=EXCLUDED.music_uid,nickname=EXCLUDED.nickname,avatar=EXCLUDED.avatar,cookie_cipher=EXCLUDED.cookie_cipher,bound_at=now(),expired=false", gid, profile.UID, profile.Nickname, safeProviderImage(musicProvider(r), profile.Avatar), cipherCookie)
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			fail(w, 409, "这个音乐平台账号已绑定其他食堂账号，请先在原账号解除绑定")
			return
		}
		a.internal(w, e)
		return
	}
	_, e = tx.Exec(ctx, "DELETE FROM "+musicAttemptTable(r)+" WHERE account_id=$1", gid)
	if e != nil {
		a.internal(w, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]string{"status": "bound"})
}
func (a *app) musicCancel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"attempt_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	_, e := a.db.Exec(r.Context(), "DELETE FROM "+musicAttemptTable(r)+" WHERE id=$1 AND account_id=$2 AND session_hash=$3", in.ID, r.Context().Value(guestKey), hash(bearer(r)))
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) musicUnbind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gid := ctx.Value(guestKey)
	tx, e := a.db.Begin(ctx)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	var id string
	e = tx.QueryRow(ctx, "SELECT id FROM accounts WHERE id=$1 FOR UPDATE", gid).Scan(&id)
	if e != nil {
		a.internal(w, e)
		return
	}
	_, e = tx.Exec(ctx, "DELETE FROM "+musicAttemptTable(r)+" WHERE account_id=$1", gid)
	if e != nil {
		a.internal(w, e)
		return
	}
	_, e = tx.Exec(ctx, "DELETE FROM "+musicBindingTable(r)+" WHERE account_id=$1", gid)
	if e != nil {
		a.internal(w, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

type musicPlaylist struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cover   string `json:"cover"`
	Count   int    `json:"track_count"`
	Created bool   `json:"created"`
	URL     string `json:"url"`
}

func (a *app) musicPlaylists(w http.ResponseWriter, r *http.Request) {
	if !a.musicReady(w) {
		return
	}
	offset := 0
	var e error
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, e = strconv.Atoi(v)
	}
	if e != nil || offset < 0 || offset > 20000 {
		fail(w, 400, "歌单页码不正确")
		return
	}
	gid := r.Context().Value(guestKey).(string)
	if !a.limits.allow("music-playlists:"+musicProvider(r)+":"+gid, 12) {
		fail(w, 429, "歌单刷新过于频繁，请稍后再试")
		return
	}
	var encrypted []byte
	var uid string
	var expired bool
	e = a.db.QueryRow(r.Context(), "SELECT cookie_cipher,music_uid,expired FROM "+musicBindingTable(r)+" WHERE account_id=$1", gid).Scan(&encrypted, &uid, &expired)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 409, "请先绑定音乐平台账号")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if expired || len(encrypted) == 0 {
		fail(w, 409, "音乐平台登录已失效，请重新扫码")
		return
	}
	cookie, e := a.music.open(encrypted, musicAAD(r, gid, "binding"))
	if e != nil {
		a.musicError(w)
		return
	}
	var result struct {
		UID   string          `json:"uid"`
		Items []musicPlaylist `json:"items"`
		More  bool            `json:"more"`
	}
	e = a.music.call(r.Context(), "/playlists", map[string]any{"cookie": json.RawMessage(cookie), "offset": offset}, &result)
	if errors.Is(e, errMusicExpired) || (e == nil && result.UID != uid) {
		_, _ = a.db.Exec(r.Context(), "UPDATE "+musicBindingTable(r)+" SET expired=true,cookie_cipher=NULL WHERE account_id=$1 AND cookie_cipher=$2", gid, encrypted)
		fail(w, 409, "音乐平台登录已失效，请重新扫码")
		return
	}
	if e != nil {
		a.musicError(w)
		return
	}
	items := []musicPlaylist{}
	for _, p := range result.Items {
		if !validMusicResource(musicProvider(r), "playlist", p.ID) {
			continue
		}
		p.Cover = safeProviderImage(musicProvider(r), p.Cover)
		p.URL = "https://music.163.com/#/playlist?id=" + p.ID
		if musicProvider(r) == "kugou" {
			p.URL = "https://www.kugou.com/"
		}
		items = append(items, p)
	}
	// Do not deliver a response from credentials that were unbound/replaced while upstream was pending.
	var stillBound bool
	e = a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM "+musicBindingTable(r)+" WHERE account_id=$1 AND cookie_cipher=$2 AND NOT expired)", gid, encrypted).Scan(&stillBound)
	if e != nil {
		a.internal(w, e)
		return
	}
	if !stillBound {
		fail(w, 409, "绑定状态已变化，请刷新")
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "more": result.More, "offset": offset})
}
func configureMusic() (*musicClient, error) {
	endpoint := os.Getenv("NCM_BRIDGE_URL")
	if endpoint == "" {
		return nil, nil
	}
	return newMusicClient(endpoint, os.Getenv("NCM_BRIDGE_TOKEN"), os.Getenv("MUSIC_ENCRYPTION_KEY"))
}

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Uses an explicitly supplied disposable PostgreSQL database and a fake provider.
// No real music credentials or accounts are involved in these isolation/race tests.
func TestMusicIntegration(t *testing.T) {
	for _, provider := range []string{"netease", "kugou"} {
		t.Run(provider, func(t *testing.T) { testMusicIntegration(t, provider) })
	}
}
func testMusicIntegration(t *testing.T, provider string) {
	dsn := os.Getenv("MUSIC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MUSIC_TEST_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(ctx, schema); e != nil {
		t.Fatal(e)
	}
	bindingTable, attemptTable := "music_bindings", "music_link_attempts"
	trackID, audioURL, playlistURL := "456", "https://m7.music.126.net/test.mp3", "https://music.163.com/#/playlist?id=123"
	if provider == "kugou" {
		bindingTable = "kugou_music_bindings"
		attemptTable = "kugou_music_link_attempts"
		trackID = strings.Repeat("A", 32) + "_1_2"
		audioURL = "https://fs.open.kugou.com/test.mp3"
		playlistURL = "https://www.kugou.com/"
	}
	var count atomic.Int64
	var code atomic.Int32
	code.Store(801)
	var expired atomic.Bool
	var beforeCheck, beforePlaylists, beforePlayback func()
	var playbackMode atomic.Int32
	var hookMu sync.Mutex
	setCheckHook := func(fn func()) { hookMu.Lock(); beforeCheck = fn; hookMu.Unlock() }
	setPlaybackHook := func(fn func()) { hookMu.Lock(); beforePlayback = fn; hookMu.Unlock() }
	setPlaylistHook := func(fn func()) { hookMu.Lock(); beforePlaylists = fn; hookMu.Unlock() }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&input)
		path := r.URL.Path
		if provider == "kugou" {
			if !strings.HasPrefix(path, "/kugou/") {
				t.Error("wrong upstream provider")
			}
			path = strings.TrimPrefix(path, "/kugou")
		}
		switch path {
		case "/qr/start":
			jsonOut(w, 200, map[string]any{"key": "fake-qr-" + strconv.FormatInt(count.Add(1), 10), "cookie": map[string]string{"deviceId": "fake-device"}, "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10})})
		case "/qr/check":
			hookMu.Lock()
			hook := beforeCheck
			hookMu.Unlock()
			if hook != nil {
				hook()
			}
			out := map[string]any{"code": code.Load()}
			if code.Load() == 803 {
				out["cookie"] = map[string]string{"MUSIC_U": "test-secret-never-return-to-browser"}
			}
			jsonOut(w, 200, out)
		case "/account":
			jsonOut(w, 200, map[string]string{"uid": "1001", "nickname": "测试音乐账号", "avatar": "https://p1.music.126.net/test.png"})
		case "/tracks":
			jsonOut(w, 200, map[string]any{"uid": "1001", "playlist_id": "123", "name": "test", "offset": 0, "total": 1, "more": false, "items": []map[string]any{{"id": trackID, "name": "test song", "artist": "test artist", "duration": 180, "cover": "https://evil.test/cover"}}})
		case "/playback":
			hookMu.Lock()
			hook := beforePlayback
			hookMu.Unlock()
			if hook != nil {
				hook()
			}
			var id string
			_ = json.Unmarshal(input["track_id"], &id)
			result := musicAudio{UID: "1001", TrackID: id, Status: "playable", URL: strings.Replace(audioURL, "https:", "http:", 1), ExpiresIn: 300}
			if playbackMode.Load() == 1 {
				result.Status = "trial"
				result.TrialStart = 60
				result.TrialEnd = 90
			}
			if playbackMode.Load() == 2 {
				result.URL = "https://evil.test/audio"
			}
			if playbackMode.Load() == 3 {
				result.Status = "unavailable"
			}
			jsonOut(w, 200, result)
		case "/playlists":
			if _, ok := input["uid"]; ok {
				t.Error("caller-supplied music UID leaked upstream")
			}
			hookMu.Lock()
			hook := beforePlaylists
			hookMu.Unlock()
			if hook != nil {
				hook()
			}
			if expired.Load() {
				jsonOut(w, 401, map[string]string{"error": "session_expired"})
				return
			}
			jsonOut(w, 200, map[string]any{"uid": "1001", "more": false, "items": []map[string]any{{"id": "123", "name": "测试歌单", "cover": "https://evil.test/image", "track_count": 2, "created": true}}})
		default:
			fail(w, 404, "unknown")
		}
	}))
	defer upstream.Close()
	a := &app{db: db, music: testMusicClient(t, upstream.URL)}
	srv := httptest.NewServer(a.routes())
	defer srv.Close()
	type reply struct {
		status int
		body   map[string]any
		raw    string
	}
	request := func(path, tok string, data any) reply {
		method := "GET"
		var body io.Reader
		if data != nil {
			method = "POST"
			b, _ := json.Marshal(data)
			body = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, srv.URL+path, body)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			return reply{status: 0, raw: e.Error()}
		}
		defer r.Body.Close()
		raw, _ := io.ReadAll(r.Body)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return reply{r.StatusCode, out, string(raw)}
	}
	expect := func(got reply, status int) {
		t.Helper()
		if got.status != status {
			t.Fatalf("HTTP %d want %d: %s", got.status, status, got.raw)
		}
		for _, secret := range []string{"test-secret-never-return-to-browser", "MUSIC_U", "fake-qr-"} {
			if strings.Contains(got.raw, secret) {
				t.Fatal("credential leaked")
			}
		}
	}
	ids := []string{token(), token()}
	tokens := []string{token(), token(), token()}
	for i, id := range ids {
		_, e = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash(token()))
		if e != nil {
			t.Fatal(e)
		}
		_, e = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test')", id, "music_"+id[:10])
		if e != nil {
			t.Fatal(e)
		}
		_, e = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 day')", hash(tokens[i]), id)
		if e != nil {
			t.Fatal(e)
		}
	}
	defer func() {
		for _, id := range ids {
			_, _ = db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			_, _ = db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	_, e = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 day')", hash(tokens[2]), ids[0])
	if e != nil {
		t.Fatal(e)
	}
	ta, tb, ta2 := tokens[0], tokens[1], tokens[2]
	base := "/api/music/" + provider
	expect(request(base, "", nil), 401)
	start := request(base+"/qr", ta, map[string]any{})
	expect(start, 201)
	attempt := start.body["attempt_id"].(string)
	expect(request(base+"/qr/check", tb, map[string]string{"attempt_id": attempt}), 410)
	expect(request(base+"/qr/check", ta2, map[string]string{"attempt_id": attempt}), 410)
	expect(request(base+"/qr/check", ta, map[string]string{"attempt_id": attempt}), 200)
	code.Store(802)
	confirm := request(base+"/qr/check", ta, map[string]string{"attempt_id": attempt})
	expect(confirm, 200)
	if confirm.body["status"] != "confirming" {
		t.Fatal("confirmation state")
	}
	code.Store(803)
	bound := request(base+"/qr/check", ta, map[string]string{"attempt_id": attempt})
	expect(bound, 200)
	if bound.body["status"] != "bound" {
		t.Fatal("binding failed")
	}
	if provider == "netease" {
		otherCipher, _ := a.music.seal([]byte(`{"token":"fake-kugou"}`), ids[0]+":kugou:binding")
		_, e = db.Exec(ctx, "INSERT INTO kugou_music_bindings(account_id,music_uid,nickname,cookie_cipher) VALUES($1,'1001','other provider',$2)", ids[0], otherCipher)
		if e != nil {
			t.Fatal(e)
		}
		expect(request("/api/music/kugou/qr/check", ta, map[string]string{"attempt_id": attempt}), 410)
		expect(request("/api/music/kugou/unbind", ta, map[string]any{}), 200)
		kept := request(base, ta, nil)
		expect(kept, 200)
		if kept.body["bound"] != true {
			t.Fatal("Kugou unbind changed NetEase")
		}
		_, e = db.Exec(ctx, "INSERT INTO kugou_music_bindings(account_id,music_uid,nickname,cookie_cipher) VALUES($1,'1001','other provider',$2)", ids[0], otherCipher)
		if e != nil {
			t.Fatal(e)
		}
	}
	var encrypted []byte
	e = db.QueryRow(ctx, "SELECT cookie_cipher FROM "+bindingTable+" WHERE account_id=$1", ids[0]).Scan(&encrypted)
	if e != nil || bytes.Contains(encrypted, []byte("test-secret")) {
		t.Fatal("credential not encrypted")
	}
	expect(request(base+"/playlists", tb, nil), 409)
	list := request(base+"/playlists?uid=9999", ta, nil)
	expect(list, 200)
	item := list.body["items"].([]any)[0].(map[string]any)
	if item["cover"] != "" || item["url"] != playlistURL {
		t.Fatal("playlist sanitization")
	}
	expect(request(base+"/tracks?playlist_id=123", tb, nil), 409)
	expect(request(base+"/tracks?playlist_id=123&offset=-1", ta, nil), 400)
	expect(request(base+"/playback", "", map[string]string{"track_id": trackID}), 401)
	expect(request(base+"/playback", tb, map[string]string{"track_id": trackID}), 409)
	expect(request(base+"/playback", ta, map[string]string{"track_id": trackID, "url": "https://evil.test"}), 400)
	tracks := request(base+"/tracks?playlist_id=123", ta, nil)
	expect(tracks, 200)
	if tracks.body["items"].([]any)[0].(map[string]any)["cover"] != "" {
		t.Fatal("unsafe cover returned")
	}
	media := request(base+"/playback", ta, map[string]string{"track_id": trackID})
	expect(media, 200)
	if media.body["url"] != audioURL {
		t.Fatal("audio URL not upgraded safely")
	}
	playbackMode.Store(1)
	media = request(base+"/playback", ta, map[string]string{"track_id": trackID})
	expect(media, 200)
	if media.body["status"] != "trial" || media.body["trial_end"] != float64(90) {
		t.Fatal("trial range not preserved")
	}
	for _, mode := range []int32{2, 3} {
		playbackMode.Store(mode)
		media = request(base+"/playback", ta, map[string]string{"track_id": trackID})
		expect(media, 200)
		if media.body["status"] != "unavailable" || media.body["url"] != "" {
			t.Fatal("invalid stream exposed")
		}
	}
	playbackMode.Store(0)
	second := request(base+"/qr", tb, map[string]any{})
	expect(second, 201)
	bAttempt := second.body["attempt_id"].(string)
	expect(request(base+"/qr/check", tb, map[string]string{"attempt_id": bAttempt}), 409)
	started, release := make(chan struct{}), make(chan struct{})
	setPlaylistHook(func() { close(started); <-release })
	done := make(chan reply, 1)
	go func() { done <- request(base+"/playlists", ta, nil) }()
	<-started
	playbackStarted, playbackRelease := make(chan struct{}), make(chan struct{})
	setPlaybackHook(func() { close(playbackStarted); <-playbackRelease })
	playbackDone := make(chan reply, 1)
	go func() { playbackDone <- request(base+"/playback", ta, map[string]string{"track_id": trackID}) }()
	<-playbackStarted
	expect(request(base+"/unbind", ta, map[string]any{}), 200)
	close(playbackRelease)
	expect(<-playbackDone, 409)
	setPlaybackHook(nil)
	close(release)
	expect(<-done, 409)
	setPlaylistHook(nil)
	if provider == "netease" {
		kept := request("/api/music/kugou", ta, nil)
		expect(kept, 200)
		if kept.body["bound"] != true {
			t.Fatal("NetEase unbind changed Kugou")
		}
	}
	expect(request(base+"/qr/check", tb, map[string]string{"attempt_id": bAttempt}), 200)
	expired.Store(true)
	expect(request(base+"/playlists", tb, nil), 409)
	st := request(base, tb, nil)
	expect(st, 200)
	if st.body["expired"] != true {
		t.Fatal("expired music session not marked")
	}
	expect(request("/api/me", tb, nil), 200)
	start = request(base+"/qr", ta, map[string]any{})
	expect(start, 201)
	attempt = start.body["attempt_id"].(string)
	expect(request(base+"/qr/cancel", ta, map[string]string{"attempt_id": attempt}), 200)
	expect(request(base+"/qr/check", ta, map[string]string{"attempt_id": attempt}), 410)
	start = request(base+"/qr", ta, map[string]any{})
	expect(start, 201)
	attempt = start.body["attempt_id"].(string)
	code.Store(800)
	expiredQR := request(base+"/qr/check", ta, map[string]string{"attempt_id": attempt})
	expect(expiredQR, 200)
	if expiredQR.body["status"] != "expired" {
		t.Fatal("QR expiry")
	}
	start = request(base+"/qr", ta, map[string]any{})
	expect(start, 201)
	attempt = start.body["attempt_id"].(string)
	code.Store(803)
	started, release = make(chan struct{}), make(chan struct{})
	setCheckHook(func() { close(started); <-release })
	go func() { done <- request(base+"/qr/check", ta, map[string]string{"attempt_id": attempt}) }()
	<-started
	expect(request(base+"/unbind", ta, map[string]any{}), 200)
	close(release)
	expect(<-done, 410)
	setCheckHook(nil)
	st = request(base, ta, nil)
	expect(st, 200)
	if st.body["bound"] != false {
		t.Fatal("late QR check restored unbound credentials")
	}
	start = request(base+"/qr", tb, map[string]any{})
	expect(start, 201)
	expect(request("/api/auth/logout", tb, map[string]any{}), 200)
	expect(request(base+"/qr/check", tb, map[string]any{"attempt_id": start.body["attempt_id"]}), 401)
	var remaining int
	_ = db.QueryRow(ctx, "SELECT count(*) FROM "+attemptTable+" WHERE account_id=$1", ids[1]).Scan(&remaining)
	if remaining != 0 {
		t.Fatal("logout did not remove pending QR")
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fakeCatalog(fn func(*http.Request) (int, string)) *catalogGateway {
	return &catalogGateway{client: &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		code, body := fn(r)
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}, cache: map[string]catalogCacheEntry{}}
}
func TestCatalogCacheAndSharedBudget(t *testing.T) {
	var calls atomic.Int32
	g := fakeCatalog(func(r *http.Request) (int, string) {
		calls.Add(1)
		if r.URL.Host != "music-api.gdstudio.xyz" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("Referer") != "" {
			t.Error("unexpected destination or credentials")
		}
		return 200, `[{"id":"1","name":"song","artist":["artist"]}]`
	})
	p := url.Values{"types": {"search"}, "source": {"netease"}, "name": {"same"}}
	for i := 0; i < 3; i++ {
		if _, e := g.get(context.Background(), p); e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cache missed")
	}
	for i := 0; i < 39; i++ {
		p.Set("name", string(rune('一'+i)))
		if _, e := g.get(context.Background(), p); e != nil {
			t.Fatal(e)
		}
	}
	p.Set("name", "overflow")
	if _, e := g.get(context.Background(), p); !errors.Is(e, errCatalogBusy) {
		t.Fatalf("expected budget rejection, got %v", e)
	}
	p.Set("name", "same")
	if _, e := g.get(context.Background(), p); e != nil {
		t.Fatal("cached result must work when budget full")
	}
	for i := range g.calls {
		g.calls[i] = time.Now().Add(-6 * time.Minute)
	}
	p.Set("name", "fresh")
	if _, e := g.get(context.Background(), p); e != nil {
		t.Fatal("rolling budget never recovered")
	}
}
func TestCatalogPartialFailureAndProvenance(t *testing.T) {
	old := gdCatalog
	defer func() { gdCatalog = old }()
	gdCatalog = fakeCatalog(func(r *http.Request) (int, string) {
		if r.URL.Query().Get("source") == "joox" {
			return 503, `{}`
		}
		return 200, `[{"id":"66842","name":"十年","artist":["陈奕迅"],"album":"黑白灰"}]`
	})
	a := &app{}
	r := httptest.NewRequest("GET", "/api/canteen/search?source=all&q=test", nil)
	r = r.WithContext(context.WithValue(r.Context(), guestKey, "test-account"))
	w := httptest.NewRecorder()
	a.canteenSearch(w, r)
	var out struct {
		Tracks  []canteenTrack        `json:"tracks"`
		Sources []catalogSourceResult `json:"sources"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Tracks) != 1 || out.Tracks[0].Source != "netease" {
		t.Fatalf("partial failure lost valid results: %s", w.Body)
	}
	if len(out.Sources) != 2 || out.Sources[0].Error == "" {
		t.Fatalf("missing source failure: %s", w.Body)
	}
}
func TestCatalogPlaybackDoesNotSubstituteUnavailableTrack(t *testing.T) {
	old := gdCatalog
	defer func() { gdCatalog = old }()
	gdCatalog = fakeCatalog(func(r *http.Request) (int, string) { return 200, `{"url":"","br":-1}` })
	a := &app{}
	r := httptest.NewRequest("POST", "/api/canteen/playback", strings.NewReader(`{"source":"joox","id":"abc+_=="}`))
	r = r.WithContext(context.WithValue(r.Context(), guestKey, "test-account"))
	w := httptest.NewRecorder()
	a.canteenPlayback(w, r)
	if w.Code != 409 || strings.Contains(w.Body.String(), `"url"`) {
		t.Fatalf("unavailable track not rejected: %s", w.Body)
	}
}
func TestCatalogURLAndIdentifierValidation(t *testing.T) {
	for _, raw := range []string{"http://m7.music.126.net/a.mp3", "https://music.126.net.evil.test/a", "https://127.0.0.1/a", "https://user:pass@m7.music.126.net/a", "https://m7.music.126.net:444/a", "file:///etc/passwd"} {
		if safeCatalogAudio(raw) {
			t.Error("unsafe URL accepted", raw)
		}
	}
	if !safeCatalogAudio("https://m701.music.126.net/a.mp3") {
		t.Fatal("valid CDN rejected")
	}
	if validCatalogID("netease", "../123") || validCatalogID("joox", "http://foo") || validCatalogID("unknown", "1") {
		t.Fatal("invalid resource accepted")
	}
	if !validCatalogID("joox", "aB_c+12==") {
		t.Fatal("JOOX id rejected")
	}
	for title, want := range map[string]string{"晴天 (原唱 周杰伦)": "翻唱/改编", "十年 (Live)": "现场版", "江南伴奏": "伴奏/纯音乐", "十年": "未标版本"} {
		if catalogVersion(title) != want {
			t.Errorf("%s mislabeled", title)
		}
	}
}
func TestCatalogDoesNotCacheDeniedRequests(t *testing.T) {
	var calls int
	g := fakeCatalog(func(r *http.Request) (int, string) { calls++; return 403, `{"error":"denied"}` })
	for i := 0; i < 2; i++ {
		if _, e := g.get(context.Background(), url.Values{"types": {"search"}}); e == nil {
			t.Fatal("denial ignored")
		}
	}
	if calls != 2 || len(g.cache) != 0 {
		t.Fatal("denied response cached")
	}
}

func TestCatalogLyricsUseLyricIDAndCache(t *testing.T) {
	var calls int
	g := fakeCatalog(func(r *http.Request) (int, string) {
		calls++
		if r.URL.Query().Get("types") == "search" {
			return 200, `[{"id":"123","lyric_id":"456","name":"song","artist":["artist"]}]`
		}
		if r.URL.Query().Get("id") != "456" {
			t.Error("wrong lyrics resource")
		}
		return 200, `{"lyric":"[00:01.00]first line","tlyric":"[00:01.00]translation"}`
	})
	tracks, _, err := g.search(context.Background(), "netease", "song", 1)
	if err != nil || len(tracks) != 1 || tracks[0].LyricID != "456" {
		t.Fatalf("lyric id lost: %+v %v", tracks, err)
	}
	old := gdCatalog
	gdCatalog = g
	defer func() { gdCatalog = old }()
	a := &app{}
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("GET", "/api/canteen/lyrics?source=netease&id=456", nil)
		r = r.WithContext(context.WithValue(r.Context(), guestKey, "test-account"))
		w := httptest.NewRecorder()
		a.canteenLyrics(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "translation") {
			t.Fatalf("lyrics missing: %s", w.Body)
		}
	}
	if calls != 2 {
		t.Fatalf("lyrics should be cached; calls=%d", calls)
	}
	for k, v := range g.cache {
		if strings.Contains(k, "types=lyric") && time.Until(v.until) < 9*time.Minute {
			t.Fatal("lyrics got short URL cache lifetime")
		}
	}
}
func TestCatalogLyricsFailureAndUnsupportedSource(t *testing.T) {
	old := gdCatalog
	defer func() { gdCatalog = old }()
	calls := 0
	gdCatalog = fakeCatalog(func(r *http.Request) (int, string) { calls++; return 503, `{}` })
	a := &app{}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/canteen/lyrics?source=unknown&id=1", 400},
		{"/api/canteen/lyrics?source=audius&id=ng9rl", 200},
		{"/api/canteen/lyrics?source=netease&id=66842", 502},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r = r.WithContext(context.WithValue(r.Context(), guestKey, "test-account"))
		w := httptest.NewRecorder()
		a.canteenLyrics(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s got %d", tc.path, w.Code)
		}
	}
	if calls != 1 {
		t.Fatal("unsupported source triggered upstream request")
	}
}

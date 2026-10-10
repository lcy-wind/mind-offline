package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testMusicClient(t *testing.T, url string) *musicClient {
	t.Helper()
	m, e := newMusicClient(url, strings.Repeat("x", 32), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestMusicEncryption(t *testing.T) {
	m := testMusicClient(t, "http://127.0.0.1:18085")
	plain := []byte(`{"MUSIC_U":"private-test-credential"}`)
	one, e := m.seal(plain, "account-a:binding")
	if e != nil {
		t.Fatal(e)
	}
	two, _ := m.seal(plain, "account-a:binding")
	if bytes.Equal(one, two) || bytes.Contains(one, plain) {
		t.Fatal("nonce/ciphertext failure")
	}
	got, e := m.open(one, "account-a:binding")
	if e != nil || !bytes.Equal(got, plain) {
		t.Fatal("decrypt failed")
	}
	if _, e = m.open(one, "account-b:binding"); e == nil {
		t.Fatal("cross-account decrypt succeeded")
	}
	if _, e = m.open(one, "account-a:qr:123"); e == nil {
		t.Fatal("cross-purpose decrypt succeeded")
	}
	one[len(one)-1] ^= 1
	if _, e = m.open(one, "account-a:binding"); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
func TestMusicTransportErrors(t *testing.T) {
	for _, category := range []string{"unauthorized", "session_expired"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 401, map[string]string{"error": category}) }))
		m := testMusicClient(t, s.URL)
		e := m.call(context.Background(), "/account", nil, &struct{}{})
		if errors.Is(e, errMusicExpired) != (category == "session_expired") {
			t.Fatal("bridge auth failure must not revoke user credentials")
		}
		s.Close()
	}
}
func TestMusicValidation(t *testing.T) {
	for _, s := range []string{"https://evil.test/a.png", "javascript:alert(1)", "https://music.126.net.evil.test/a", "https://user:pass@p1.music.126.net/a"} {
		if safeMusicImage(s) != "" {
			t.Fatalf("untrusted image accepted %s", s)
		}
	}
	if safeMusicImage("https://p1.music.126.net/avatar.png") == "" {
		t.Fatal("valid image rejected")
	}
	for _, id := range []string{"0", "-1", "1<script>", "1234567890123456789"} {
		if validMusicID(id) {
			t.Fatal("invalid ID")
		}
	}
	if !validMusicID("123456789") {
		t.Fatal("valid ID")
	}
	if _, e := newMusicClient("https://evil.test", strings.Repeat("x", 32), strings.Repeat("x", 32)); e == nil {
		t.Fatal("non-loopback allowed")
	}
}

func TestSafeAudioURL(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/a.mp3", "https://music.126.net.evil.test/a", "https://m7.music.126.net:8080/a", "file:///a", "javascript:alert(1)", "https://user:pass@m7.music.126.net/a"} {
		if safeAudioURL(raw) != "" {
			t.Fatal("unsafe audio URL", raw)
		}
	}
	if safeAudioURL("http://m7.music.126.net/a.mp3?x=1") != "https://m7.music.126.net/a.mp3?x=1" {
		t.Fatal("valid HTTPS conversion")
	}
}

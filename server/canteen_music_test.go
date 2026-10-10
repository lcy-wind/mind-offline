package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type canteenTransport func(*http.Request) (*http.Response, error)

func (f canteenTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCanteenOnlyPublicPlayableTracks(t *testing.T) {
	client := &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.audius.co" || r.URL.Query().Get("query") != "周五 & jazz" || r.URL.Query().Get("offset") != "24" {
			t.Fatalf("unexpected request %s", r.URL)
		}
		for _, h := range []string{"Authorization", "Cookie"} {
			if r.Header.Get(h) != "" {
				t.Fatal("credential forwarded")
			}
		}
		body := `{"data":[
   {"id":"good1","title":"Friday","user":{"name":"Artist"},"is_available":true,"is_streamable":true,"access":{"stream":true}},
   {"id":"paid","is_available":true,"is_streamable":true,"is_stream_gated":true,"access":{"stream":true}},
   {"id":"broken","is_available":true,"is_streamable":false,"access":{"stream":true}},
   {"id":"private","is_available":true,"is_streamable":true,"is_unlisted":true,"access":{"stream":true}},
   {"id":"denied","is_available":true,"is_streamable":true,"access":{"stream":false}},
   {"id":"../evil","is_available":true,"is_streamable":true,"access":{"stream":true}}
  ]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	rows, more, err := searchCanteen(context.Background(), client, "周五 & jazz", 24)
	if err != nil || more || len(rows) != 1 || rows[0].ID != "good1" || rows[0].Artist != "Artist" || rows[0].URL != audiusBase+"/tracks/good1/stream?app_name=mind-offline" {
		t.Fatalf("rows=%+v more=%v err=%v", rows, more, err)
	}
}
func TestCanteenUpstreamFailure(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{403, `{"error":"denied"}`}, {200, `broken json`}} {
		client := &http.Client{Transport: canteenTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})}
		if _, _, err := searchCanteen(context.Background(), client, "lofi", 0); err == nil {
			t.Fatal("upstream error was hidden")
		}
	}
}

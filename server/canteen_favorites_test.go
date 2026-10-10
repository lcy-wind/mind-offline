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

func TestNormalizeFavorite(t *testing.T) {
	track, ok := normalizeFavorite(canteenTrack{ID: "66842", Source: "netease", Name: " 十年 (Live) ", Artist: "陈奕迅", URL: "https://example.test/expiring?token=secret", Version: "原唱"})
	if !ok || track.URL != "" || track.Version != "现场版" || track.LyricID != "66842" {
		t.Fatalf("invalid metadata normalization: %+v", track)
	}
	for _, v := range []canteenTrack{{Source: "unknown", ID: "1", Name: "x"}, {Source: "netease", ID: "1", Name: strings.Repeat("a", 301)}, {Source: "netease", ID: "1", Name: "x", Duration: -1}} {
		if _, ok := normalizeFavorite(v); ok {
			t.Fatal("bad favorite accepted")
		}
	}
}
func TestCanteenFavoritesIntegration(t *testing.T) {
	dsn := os.Getenv("FAVORITES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL database")
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
	tokens := []string{token(), token()}
	defer func() {
		for _, id := range ids {
			db.Exec(ctx, "DELETE FROM accounts WHERE id=$1", id)
			db.Exec(ctx, "DELETE FROM guests WHERE id=$1", id)
		}
	}()
	for i, id := range ids {
		if _, err = db.Exec(ctx, "INSERT INTO guests(id,token_hash) VALUES($1,$2)", id, hash("guest"+id)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO accounts(id,username,password_hash) VALUES($1,$2,'test-only')", id, "favorites_test_"+id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "INSERT INTO customer_sessions(token_hash,account_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash(tokens[i]), id); err != nil {
			t.Fatal(err)
		}
	}
	a := &app{db: db}
	handler := a.routes()
	call := func(method, auth string, body any, status int) []canteenTrack {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "/api/canteen/favorites", bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s expected %d got %d: %s", method, status, w.Code, w.Body)
		}
		var result struct {
			Tracks []canteenTrack `json:"tracks"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		return result.Tracks
	}
	song := canteenTrack{ID: "66842", Source: "netease", Name: "十年", Artist: "陈奕迅", URL: "https://example.test/private-token"}
	save := func(auth string, v canteenTrack, favorite bool) {
		call(http.MethodPost, auth, map[string]any{"track": v, "favorite": favorite}, 200)
	}
	call("GET", "", nil, 401)
	call("POST", tokens[0], map[string]any{"track": song}, 400)
	call("POST", tokens[0], map[string]any{"track": song, "favorite": true, "account_id": ids[1]}, 400)
	save(tokens[0], song, true)
	save(tokens[0], song, true)
	rows := call("GET", tokens[0], nil, 200)
	if len(rows) != 1 || rows[0].URL != "" {
		t.Fatal("duplicate or signed URL persisted")
	}
	if len(call("GET", tokens[1], nil, 200)) != 0 {
		t.Fatal("favorites leaked across accounts")
	}
	save(tokens[1], song, false)
	if len(call("GET", tokens[0], nil, 200)) != 1 {
		t.Fatal("other account removed favorite")
	}
	save(tokens[1], song, true)
	otherSource := song
	otherSource.Source = "joox"
	save(tokens[0], otherSource, true)
	if len(call("GET", tokens[0], nil, 200)) != 2 {
		t.Fatal("provider ids collided")
	}
	save(tokens[0], song, false)
	save(tokens[0], song, false)
	if len(call("GET", tokens[1], nil, 200)) != 1 {
		t.Fatal("removal affected another account")
	}
	rows = call("GET", tokens[0], nil, 200)
	if len(rows) != 1 || rows[0].Source != "joox" {
		t.Fatal("wrong source removed")
	}
	// A fresh app instance reads persisted favorites, independent of process memory.
	handler = (&app{db: db}).routes()
	if len(call("GET", tokens[1], nil, 200)) != 1 {
		t.Fatal("favorites not persisted")
	}
	db.Exec(ctx, "UPDATE accounts SET disabled=true WHERE id=$1", ids[1])
	call("GET", tokens[1], nil, 401)
	db.Exec(ctx, "DELETE FROM customer_sessions WHERE account_id=$1", ids[0])
	call("GET", tokens[0], nil, 401)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const audiusBase = "https://api.audius.co/v1"
const canteenPageSize = 24

var audiusClient = &http.Client{Timeout: 10 * time.Second}
var audiusID = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

type canteenTrack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	Genre    string `json:"genre"`
	Duration int    `json:"duration"`
	URL      string `json:"url"`
}
type audiusTrack struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Genre      string `json:"genre"`
	Duration   int    `json:"duration"`
	Available  bool   `json:"is_available"`
	Streamable bool   `json:"is_streamable"`
	Gated      bool   `json:"is_stream_gated"`
	Unlisted   bool   `json:"is_unlisted"`
	Deleted    bool   `json:"is_delete"`
	Access     struct {
		Stream bool `json:"stream"`
	} `json:"access"`
	User struct {
		Name string `json:"name"`
	} `json:"user"`
}

func searchCanteen(ctx context.Context, client *http.Client, q string, offset int) ([]canteenTrack, bool, error) {
	values := url.Values{"query": {q}, "limit": {strconv.Itoa(canteenPageSize)}, "offset": {strconv.Itoa(offset)}, "app_name": {"mind-offline"}}
	req, err := http.NewRequestWithContext(ctx, "GET", audiusBase+"/tracks/search?"+values.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, false, fmt.Errorf("music upstream status %d", res.StatusCode)
	}
	var payload struct {
		Data []audiusTrack `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, false, err
	}
	tracks := make([]canteenTrack, 0, len(payload.Data))
	for _, t := range payload.Data {
		if !audiusID.MatchString(t.ID) || !t.Available || !t.Streamable || t.Gated || t.Unlisted || t.Deleted || !t.Access.Stream {
			continue
		}
		tracks = append(tracks, canteenTrack{ID: t.ID, Name: t.Title, Artist: t.User.Name, Genre: t.Genre, Duration: t.Duration, URL: audiusBase + "/tracks/" + t.ID + "/stream?app_name=mind-offline"})
	}
	return tracks, len(payload.Data) == canteenPageSize, nil
}

func (a *app) canteenSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = "lofi"
	}
	offset := 0
	var err error
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if utf8.RuneCountInString(q) > 80 || err != nil || offset < 0 || offset > 2400 {
		fail(w, 400, "搜索词或页码不正确")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("canteen-search:"+owner, 20) {
		fail(w, 429, "搜歌太快啦，请稍后再试")
		return
	}
	tracks, more, err := searchCanteen(r.Context(), audiusClient, q, offset)
	if err != nil {
		fail(w, 502, "曲库暂时没有回应，请稍后重试")
		return
	}
	jsonOut(w, 200, map[string]any{"tracks": tracks, "has_more": more, "next_offset": offset + canteenPageSize, "source": "Audius"})
}

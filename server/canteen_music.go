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
	"time"
)

const audiusBase = "https://api.audius.co/v1"
const canteenPageSize = 24

var audiusClient = &http.Client{Timeout: 10 * time.Second}
var audiusID = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

type canteenTrack struct {
	LyricID  string `json:"lyric_id,omitempty"`
	Source   string `json:"source"`
	Version  string `json:"version"`
	Album    string `json:"album"`
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
		tracks = append(tracks, canteenTrack{Source: "audius", Version: catalogVersion(t.Title), ID: t.ID, Name: t.Title, Artist: t.User.Name, Genre: t.Genre, Duration: t.Duration, URL: audiusBase + "/tracks/" + t.ID + "/stream?app_name=mind-offline"})
	}
	return tracks, len(payload.Data) == canteenPageSize, nil
}

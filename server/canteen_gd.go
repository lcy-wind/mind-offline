package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const gdBase = "https://music-api.gdstudio.xyz/api.php"
const gdPageSize = 20

var errCatalogBusy = errors.New("catalog request budget exhausted")
var gdResourceID = regexp.MustCompile(`^[A-Za-z0-9_+=-]{1,100}$`)
var gdNumberID = regexp.MustCompile(`^[0-9]{1,20}$`)

type catalogCacheEntry struct {
	body  []byte
	until time.Time
}
type catalogGateway struct {
	client *http.Client
	mu     sync.Mutex
	calls  []time.Time
	cache  map[string]catalogCacheEntry
}

var gdCatalog = &catalogGateway{client: &http.Client{Timeout: 8 * time.Second}, cache: make(map[string]catalogCacheEntry)}

// Shared, rolling request budget covers every user, search source and URL lookup.
// 40 / 5 minutes leaves headroom below the provider's documented 50 limit.
func (g *catalogGateway) get(ctx context.Context, params url.Values) ([]byte, error) {
	key := params.Encode()
	now := time.Now()
	g.mu.Lock()
	if cached, ok := g.cache[key]; ok && now.Before(cached.until) {
		g.mu.Unlock()
		return cached.body, nil
	}
	start := 0
	for start < len(g.calls) && now.Sub(g.calls[start]) >= 5*time.Minute {
		start++
	}
	g.calls = g.calls[start:]
	if len(g.calls) >= 40 {
		g.mu.Unlock()
		return nil, errCatalogBusy
	}
	g.calls = append(g.calls, now)
	g.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, "GET", gdBase+"?"+key, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mind-offline/1.0 (+https://mind-offline.duckdns.org)")
	res, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("catalog status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || !json.Valid(body) {
		return nil, errors.New("invalid catalog response")
	}
	ttl := 2 * time.Minute
	if params.Get("types") == "search" {
		var items []json.RawMessage
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			ttl = 15 * time.Second
		}
	} else {
		var item struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &item); err != nil {
			return nil, err
		}
		if item.URL == "" {
			ttl = 15 * time.Second
		}
	}
	// Keep cache memory below 16 MiB even if an upstream response is unusually large.
	if len(body) > 64<<10 {
		return body, nil
	}
	g.mu.Lock()
	if g.cache == nil {
		g.cache = make(map[string]catalogCacheEntry)
	}
	if len(g.cache) >= 256 {
		for k, v := range g.cache {
			if now.After(v.until) {
				delete(g.cache, k)
			}
		}
	}
	if len(g.cache) >= 256 {
		for k := range g.cache {
			delete(g.cache, k)
			break
		}
	}
	g.cache[key] = catalogCacheEntry{body: body, until: time.Now().Add(ttl)}
	g.mu.Unlock()
	return body, nil
}

func catalogVersion(name string) string {
	n := strings.ToLower(name)
	for _, word := range []string{"伴奏", "纯音乐", "純音樂", "instrumental"} {
		if strings.Contains(n, word) {
			return "伴奏/纯音乐"
		}
	}
	for _, word := range []string{"翻唱", "cover", "原唱", "改编", "改編", "女声版", "女聲版"} {
		if strings.Contains(n, word) {
			return "翻唱/改编"
		}
	}
	for _, word := range []string{"live", "现场", "現場"} {
		if strings.Contains(n, word) {
			return "现场版"
		}
	}
	for _, word := range []string{"remix", "dj版", "混音"} {
		if strings.Contains(n, word) {
			return "混音版"
		}
	}
	return "未标版本"
}
func validCatalogID(source, id string) bool {
	switch source {
	case "netease":
		return gdNumberID.MatchString(id)
	case "joox":
		return gdResourceID.MatchString(id)
	case "audius":
		return audiusID.MatchString(id)
	}
	return false
}
func (g *catalogGateway) search(ctx context.Context, source, q string, page int) ([]canteenTrack, bool, error) {
	body, err := g.get(ctx, url.Values{"types": {"search"}, "source": {source}, "name": {q}, "count": {strconv.Itoa(gdPageSize)}, "pages": {strconv.Itoa(page)}})
	if err != nil {
		return nil, false, err
	}
	var items []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Artist []string `json:"artist"`
		Album  string   `json:"album"`
	}
	if err = json.Unmarshal(body, &items); err != nil {
		return nil, false, err
	}
	tracks := make([]canteenTrack, 0, len(items))
	seen := map[string]bool{}
	for _, t := range items {
		if !validCatalogID(source, t.ID) || t.Name == "" || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		tracks = append(tracks, canteenTrack{ID: t.ID, Source: source, Name: t.Name, Artist: strings.Join(t.Artist, " / "), Album: t.Album, Version: catalogVersion(t.Name)})
	}
	return tracks, len(items) >= gdPageSize, nil
}

type catalogSourceResult struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
	Error  string `json:"error,omitempty"`
}

func catalogError(err error) string {
	if errors.Is(err, errCatalogBusy) {
		return "曲库请求较多，请稍后再试"
	}
	return "该来源暂时没有回应"
}
func (a *app) canteenSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		fail(w, 400, "请输入歌名或歌手")
		return
	}
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "all"
	}
	page := 1
	var err error
	if v := r.URL.Query().Get("page"); v != "" {
		page, err = strconv.Atoi(v)
	}
	if utf8.RuneCountInString(q) > 80 || err != nil || page < 1 || page > 100 || (source != "all" && source != "netease" && source != "joox" && source != "audius") {
		fail(w, 400, "搜索词、来源或页码不正确")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("canteen-search:"+owner, 15) {
		fail(w, 429, "搜歌太快啦，请稍后再试")
		return
	}
	sources := []string{source}
	if source == "all" {
		sources = []string{"netease", "joox"}
	}
	type reply struct {
		tracks []canteenTrack
		more   bool
		err    error
		source string
	}
	replies := make(chan reply, len(sources))
	for _, s := range sources {
		go func(s string) {
			var tracks []canteenTrack
			var more bool
			var err error
			if s == "audius" {
				tracks, more, err = searchCanteen(r.Context(), audiusClient, q, (page-1)*canteenPageSize)
			} else {
				tracks, more, err = gdCatalog.search(r.Context(), s, q, page)
			}
			replies <- reply{tracks, more, err, s}
		}(s)
	}
	out := make([]canteenTrack, 0)
	statuses := make([]catalogSourceResult, 0, len(sources))
	more := false
	success := 0
	for range sources {
		v := <-replies
		s := catalogSourceResult{Source: v.source, Count: len(v.tracks)}
		if v.err != nil {
			s.Error = catalogError(v.err)
		} else {
			success++
			out = append(out, v.tracks...)
			more = more || v.more
		}
		statuses = append(statuses, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		rank := func(t canteenTrack) int {
			n := 0
			if t.Version != "未标版本" {
				n += 2
			}
			if t.Source == "joox" {
				n++
			}
			return n
		}
		return rank(out[i]) < rank(out[j])
	})
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Source < statuses[j].Source })
	if success == 0 {
		fail(w, 502, "曲库暂时繁忙，请稍后重试")
		return
	}
	jsonOut(w, 200, map[string]any{"tracks": out, "has_more": more, "next_page": page + 1, "sources": statuses})
}

// Audio goes directly to known public music CDNs. This endpoint is not a proxy.
func safeCatalogAudio(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{"music.126.net", "music.tc.qq.com", "stream.qqmusic.qq.com", "joox.com"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
func (a *app) canteenPlayback(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !validCatalogID(input.Source, input.ID) {
		fail(w, 400, "歌曲编号或来源不正确")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("canteen-play:"+owner, 20) {
		fail(w, 429, "切歌太快啦，请稍后再试")
		return
	}
	if input.Source == "audius" {
		jsonOut(w, 200, map[string]string{"url": audiusBase + "/tracks/" + input.ID + "/stream?app_name=mind-offline"})
		return
	}
	body, err := gdCatalog.get(r.Context(), url.Values{"types": {"url"}, "source": {input.Source}, "id": {input.ID}, "br": {"128"}})
	if err != nil {
		fail(w, 502, catalogError(err))
		return
	}
	var result struct {
		URL     string `json:"url"`
		Bitrate int    `json:"br"`
	}
	if json.Unmarshal(body, &result) != nil || result.URL == "" {
		fail(w, 409, "这个来源暂无可播放地址，试试换源找同曲")
		return
	}
	if !safeCatalogAudio(result.URL) {
		fail(w, 502, "这个来源返回的音频地址暂不支持，请换源")
		return
	}
	jsonOut(w, 200, map[string]any{"url": result.URL, "bitrate": result.Bitrate})
}

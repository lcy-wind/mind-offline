package main

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type boundMusic struct {
	owner, uid        string
	encrypted, cookie []byte
}

func (a *app) boundMusic(w http.ResponseWriter, r *http.Request) (*boundMusic, bool) {
	if !a.musicReady(w) {
		return nil, false
	}
	b := &boundMusic{owner: r.Context().Value(guestKey).(string)}
	var expired bool
	e := a.db.QueryRow(r.Context(), "SELECT cookie_cipher,music_uid,expired FROM "+musicBindingTable(r)+" WHERE account_id=$1", b.owner).Scan(&b.encrypted, &b.uid, &expired)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 409, "请先绑定音乐平台账号")
		return nil, false
	}
	if e != nil {
		a.internal(w, e)
		return nil, false
	}
	if expired || len(b.encrypted) == 0 {
		fail(w, 409, "音乐平台登录已失效，请重新扫码")
		return nil, false
	}
	b.cookie, e = a.music.open(b.encrypted, musicAAD(r, b.owner, "binding"))
	if e != nil {
		a.musicError(w)
		return nil, false
	}
	return b, true
}
func (a *app) checkMusicReply(w http.ResponseWriter, r *http.Request, b *boundMusic, e error, uid string) bool {
	if errors.Is(e, errMusicExpired) || (e == nil && uid != b.uid) {
		_, _ = a.db.Exec(r.Context(), "UPDATE "+musicBindingTable(r)+" SET expired=true,cookie_cipher=NULL WHERE account_id=$1 AND cookie_cipher=$2", b.owner, b.encrypted)
		fail(w, 409, "音乐平台登录已失效，请重新扫码")
		return false
	}
	if e != nil {
		a.musicError(w)
		return false
	}
	var bound, session bool
	e = a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM "+musicBindingTable(r)+" WHERE account_id=$1 AND cookie_cipher=$2 AND NOT expired),EXISTS(SELECT 1 FROM customer_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.account_id=$1 AND s.token_hash=$3 AND s.expires_at>now() AND NOT a.disabled)", b.owner, b.encrypted, hash(bearer(r))).Scan(&bound, &session)
	if e != nil {
		a.internal(w, e)
		return false
	}
	if !session {
		fail(w, 401, "请登录后继续")
		return false
	}
	if !bound {
		fail(w, 409, "绑定状态已变化，请刷新")
		return false
	}
	return true
}

type musicTrack struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Cover    string  `json:"cover"`
	Duration float64 `json:"duration"`
	Fee      int     `json:"fee"`
}

func (a *app) musicTracks(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("playlist_id")
	offset := 0
	var e error
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, e = strconv.Atoi(v)
	}
	if !validMusicResource(musicProvider(r), "playlist", id) || e != nil || offset < 0 || offset > 20000 {
		fail(w, 400, "歌单编号或页码不正确")
		return
	}
	b, ok := a.boundMusic(w, r)
	if !ok {
		return
	}
	if !a.limits.allow("music-tracks:"+musicProvider(r)+":"+b.owner, 20) {
		fail(w, 429, "歌曲列表刷新过于频繁，请稍后重试")
		return
	}
	var result struct {
		UID        string       `json:"uid"`
		PlaylistID string       `json:"playlist_id"`
		Name       string       `json:"name"`
		Items      []musicTrack `json:"items"`
		Total      int          `json:"total"`
		Offset     int          `json:"offset"`
		More       bool         `json:"more"`
	}
	e = a.music.call(r.Context(), "/tracks", map[string]any{"cookie": json.RawMessage(b.cookie), "playlist_id": id, "offset": offset}, &result)
	if !a.checkMusicReply(w, r, b, e, result.UID) {
		return
	}
	if result.PlaylistID != id {
		a.musicError(w)
		return
	}
	items := []musicTrack{}
	for _, t := range result.Items {
		if !validMusicResource(musicProvider(r), "track", t.ID) {
			continue
		}
		t.Cover = safeProviderImage(musicProvider(r), t.Cover)
		items = append(items, t)
	}
	result.Items = items
	jsonOut(w, 200, result)
}
func safeAudioURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(u.Hostname()), ".music.126.net") {
		return ""
	}
	u.Scheme = "https"
	return u.String()
}

type musicAudio struct {
	UID        string  `json:"uid"`
	TrackID    string  `json:"track_id"`
	Status     string  `json:"status"`
	URL        string  `json:"url"`
	Message    string  `json:"message"`
	ExpiresIn  int     `json:"expires_in"`
	TrialStart float64 `json:"trial_start"`
	TrialEnd   float64 `json:"trial_end"`
}

func (a *app) musicPlayback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"track_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validMusicResource(musicProvider(r), "track", in.ID) {
		fail(w, 400, "歌曲编号不正确")
		return
	}
	b, ok := a.boundMusic(w, r)
	if !ok {
		return
	}
	if !a.limits.allow("music-play:"+musicProvider(r)+":"+b.owner, 30) {
		fail(w, 429, "切歌过于频繁，请稍后重试")
		return
	}
	var out musicAudio
	e := a.music.call(r.Context(), "/playback", map[string]any{"cookie": json.RawMessage(b.cookie), "track_id": in.ID}, &out)
	if !a.checkMusicReply(w, r, b, e, out.UID) {
		return
	}
	if out.TrackID != in.ID {
		a.musicError(w)
		return
	}
	out.URL = safeProviderAudio(musicProvider(r), out.URL)
	if out.Status != "playable" && out.Status != "trial" {
		out.Status = "unavailable"
		out.URL = ""
	}
	if out.Status == "trial" && (out.TrialStart < 0 || out.TrialEnd <= out.TrialStart || out.TrialEnd-out.TrialStart > 600) {
		out.URL = ""
	}
	if out.URL == "" {
		out.Status = "unavailable"
		out.Message = "当前歌曲无法在食堂播放，可能受权限、地区、版权或音源格式限制。"
	}
	jsonOut(w, 200, out)
}

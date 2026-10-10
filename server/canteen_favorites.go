package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
)

func normalizeFavorite(t canteenTrack) (canteenTrack, bool) {
	t.Name = strings.TrimSpace(t.Name)
	if !validCatalogID(t.Source, t.ID) || t.Name == "" || utf8.RuneCountInString(t.Name) > 300 || utf8.RuneCountInString(t.Artist) > 300 || utf8.RuneCountInString(t.Album) > 300 || utf8.RuneCountInString(t.Genre) > 100 || t.Duration < 0 || t.Duration > 86400 {
		return canteenTrack{}, false
	}
	if !validCatalogID(t.Source, t.LyricID) {
		t.LyricID = t.ID
	}
	t.URL = ""
	t.Version = catalogVersion(t.Name)
	return t, true
}
func (a *app) canteenFavorites(w http.ResponseWriter, r *http.Request) {
	owner := r.Context().Value(guestKey).(string)
	rows, err := a.db.Query(r.Context(), "SELECT track FROM canteen_music_favorites WHERE account_id=$1 ORDER BY created_at DESC, source, track_id LIMIT 1000", owner)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	out := make([]canteenTrack, 0)
	for rows.Next() {
		var raw []byte
		var track canteenTrack
		if err = rows.Scan(&raw); err != nil {
			a.internal(w, err)
			return
		}
		if err = json.Unmarshal(raw, &track); err != nil {
			a.internal(w, err)
			return
		}
		out = append(out, track)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, 200, map[string]any{"tracks": out})
}
func (a *app) saveCanteenFavorite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Track    canteenTrack `json:"track"`
		Favorite *bool        `json:"favorite"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Favorite == nil || !validCatalogID(input.Track.Source, input.Track.ID) {
		fail(w, 400, "收藏参数不正确")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("canteen-favorite:"+owner, 60) {
		fail(w, 429, "收藏操作太快啦，请稍后再试")
		return
	}
	if !*input.Favorite {
		_, err := a.db.Exec(r.Context(), "DELETE FROM canteen_music_favorites WHERE account_id=$1 AND source=$2 AND track_id=$3", owner, input.Track.Source, input.Track.ID)
		if err != nil {
			a.internal(w, err)
			return
		}
		jsonOut(w, 200, map[string]bool{"favorite": false})
		return
	}
	track, ok := normalizeFavorite(input.Track)
	if !ok {
		fail(w, 400, "歌曲信息不正确")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize additions per account so concurrent requests cannot exceed the cap.
	var id string
	if err = tx.QueryRow(r.Context(), "SELECT id FROM accounts WHERE id=$1 FOR UPDATE", owner).Scan(&id); err != nil {
		a.internal(w, err)
		return
	}
	var count int
	var exists bool
	err = tx.QueryRow(r.Context(), "SELECT count(*),COALESCE(bool_or(source=$2 AND track_id=$3),false) FROM canteen_music_favorites WHERE account_id=$1", owner, track.Source, track.ID).Scan(&count, &exists)
	if err != nil {
		a.internal(w, err)
		return
	}
	if count >= 1000 && !exists {
		fail(w, 409, "收藏已满 1000 首，先给喜欢的歌腾点位置吧")
		return
	}
	data, _ := json.Marshal(track)
	_, err = tx.Exec(r.Context(), "INSERT INTO canteen_music_favorites(account_id,source,track_id,track) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,source,track_id) DO UPDATE SET track=EXCLUDED.track", owner, track.Source, track.ID, data)
	if err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]bool{"favorite": true})
}

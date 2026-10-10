package main

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const musicProviderKey key = "music-provider"

func musicProvider(r *http.Request) string {
	if r.Context().Value(musicProviderKey) == "kugou" {
		return "kugou"
	}
	return "netease"
}
func (a *app) withMusicProvider(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.PathValue("provider")
		if p != "netease" && p != "kugou" {
			fail(w, 404, "音乐平台不存在")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), musicProviderKey, p)))
	}
}
func musicBindingTable(r *http.Request) string {
	if musicProvider(r) == "kugou" {
		return "kugou_music_bindings"
	}
	return "music_bindings"
}
func musicAttemptTable(r *http.Request) string {
	if musicProvider(r) == "kugou" {
		return "kugou_music_link_attempts"
	}
	return "music_link_attempts"
}
func musicAAD(r *http.Request, owner, kind string) string {
	if musicProvider(r) == "kugou" {
		return owner + ":kugou:" + kind
	}
	return owner + ":" + kind
}

var kgPlaylistID = regexp.MustCompile(`^(?:[0-9]{1,18}|g_[A-Za-z0-9]{1,64})$`)
var kgTrackID = regexp.MustCompile(`^[a-fA-F0-9]{32}(?:_[0-9]{1,18}_[0-9]{1,18})?$`)

func validMusicResource(provider, kind, id string) bool {
	if provider != "kugou" {
		return validMusicID(id)
	}
	if kind == "track" {
		return kgTrackID.MatchString(id)
	}
	return kgPlaylistID.MatchString(id)
}
func safeProviderImage(provider, raw string) string {
	if provider != "kugou" {
		return safeMusicImage(raw)
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, ".kugou.com") || strings.HasSuffix(host, ".kugou.net") {
		return u.String()
	}
	return ""
}
func safeProviderAudio(provider, raw string) string {
	if provider != "kugou" {
		return safeAudioURL(raw)
	}
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, ".kugou.com") && !strings.HasSuffix(host, ".kugou.net") {
		return ""
	}
	u.Scheme = "https"
	return u.String()
}

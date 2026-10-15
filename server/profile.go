package main

import (
	"bytes"
	"errors"
	"github.com/jackc/pgx/v5"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

type userProfile struct {
	Nickname  string `json:"nickname"`
	MBTI      string `json:"mbti"`
	AvatarURL string `json:"avatar_url"`
}

func avatarURL(id, version string) string {
	if version == "" {
		return ""
	}
	return "/api/avatars/" + id + "?v=" + version
}
func (a *app) profile(id string, r *http.Request) (p userProfile, err error) {
	var version string
	err = a.db.QueryRow(r.Context(), "SELECT COALESCE(NULLIF(a.nickname,''),a.username),a.mbti,COALESCE(v.version,'') FROM accounts a LEFT JOIN account_avatars v ON v.account_id=a.id WHERE a.id=$1", id).Scan(&p.Nickname, &p.MBTI, &version)
	p.AvatarURL = avatarURL(id, version)
	return
}
func validProfile(p userProfile) bool {
	if utf8.RuneCountInString(p.Nickname) < 1 || utf8.RuneCountInString(p.Nickname) > 24 {
		return false
	}
	for _, c := range p.Nickname {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) {
			return false
		}
	}
	if p.MBTI != "" {
		if _, ok := findHealingRole(p.MBTI); !ok {
			return false
		}
	}
	return true
}
func (a *app) saveProfile(w http.ResponseWriter, r *http.Request) {
	var p userProfile
	if !decode(w, r, &p) {
		return
	}
	p.Nickname = strings.TrimSpace(p.Nickname)
	p.MBTI = strings.ToUpper(strings.TrimSpace(p.MBTI))
	if !validProfile(p) {
		fail(w, 400, "昵称需为1–24字，请选择有效MBTI或暂不设置")
		return
	}
	id := r.Context().Value(guestKey).(string)
	if !a.limits.allow("profile:"+id, 20) {
		fail(w, 429, "修改太快啦，稍后再试")
		return
	}
	_, err := a.db.Exec(r.Context(), "UPDATE accounts SET nickname=$1,mbti=$2 WHERE id=$3", p.Nickname, p.MBTI, id)
	if err != nil {
		a.internal(w, err)
		return
	}
	a.lobby.notify(-1)
	a.me(w, r)
}

var avatarSlots = make(chan struct{}, 2)

func normalizeAvatar(data []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 || cfg.Width*cfg.Height > 1048576 {
		return nil, errors.New("请选择不超过100万像素的JPG或PNG图片")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("图片损坏，请换一张")
	}
	b := img.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	left, top := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	out := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			r, g, bl, a := img.At(left+x*side/256, top+y*side/256).RGBA()
			out.SetRGBA(x, y, color.RGBA{uint8((r + 65535 - a) >> 8), uint8((g + 65535 - a) >> 8), uint8((bl + 65535 - a) >> 8), 255})
		}
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, out, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	if encoded.Len() > 131072 {
		return nil, errors.New("图片过于复杂，请换一张")
	}
	return encoded.Bytes(), nil
}
func (a *app) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("avatar:"+owner, 10) {
		fail(w, 429, "头像换得太快了，稍后再试")
		return
	}
	select {
	case avatarSlots <- struct{}{}:
		defer func() { <-avatarSlots }()
	default:
		fail(w, 429, "正在处理图片，稍后再试")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, 400, "图片不能超过2MB")
		return
	}
	data, err = normalizeAvatar(data)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	_, err = a.db.Exec(r.Context(), "INSERT INTO account_avatars(account_id,jpeg,version) VALUES($1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET jpeg=EXCLUDED.jpeg,version=EXCLUDED.version", owner, data, hash(string(data)))
	if err != nil {
		a.internal(w, err)
		return
	}
	a.lobby.notify(-1)
	a.me(w, r)
}
func (a *app) removeAvatar(w http.ResponseWriter, r *http.Request) {
	_, err := a.db.Exec(r.Context(), "DELETE FROM account_avatars WHERE account_id=$1", r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	a.lobby.notify(-1)
	a.me(w, r)
}
func (a *app) avatar(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var version string
	err := a.db.QueryRow(r.Context(), "SELECT jpeg,version FROM account_avatars WHERE account_id=$1", r.PathValue("id")).Scan(&data, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("ETag", `"`+version+`"`)
	w.Write(data)
}

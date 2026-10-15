package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestProfileValidation(t *testing.T) {
	for _, p := range []userProfile{{Nickname: "摸鱼研究员", MBTI: "ENFP"}, {Nickname: "cole", MBTI: ""}} {
		if !validProfile(p) {
			t.Fatal("valid profile rejected")
		}
	}
	for _, p := range []userProfile{{Nickname: ""}, {Nickname: strings.Repeat("鱼", 25)}, {Nickname: "冒充\u202ecole"}, {Nickname: "名字\n换行"}, {Nickname: "用户", MBTI: "XXXX"}} {
		if validProfile(p) {
			t.Fatal("invalid profile accepted")
		}
	}
}
func TestAvatarNormalization(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	img.Set(200, 100, color.NRGBA{R: 255, A: 255})
	var input bytes.Buffer
	png.Encode(&input, img)
	data, err := normalizeAvatar(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil || out.Bounds().Dx() != 256 || out.Bounds().Dy() != 256 || len(data) > 131072 {
		t.Fatal("avatar bounds/format")
	}
	for _, b := range [][]byte{[]byte("<svg onload='alert(1)'/>"), []byte("bad data")} {
		if _, err = normalizeAvatar(b); err == nil {
			t.Fatal("accepted non-image")
		}
	}
	var tooLarge bytes.Buffer
	png.Encode(&tooLarge, image.NewRGBA(image.Rect(0, 0, 2049, 1)))
	if _, err = normalizeAvatar(tooLarge.Bytes()); err == nil {
		t.Fatal("accepted oversized dimensions")
	}
}

package main

import "testing"

func TestBeadValidation(t *testing.T) {
	d := beadData{Name: "拼豆", Size: 16, Mode: "free", Pixels: make([]int, 256)}
	if !validateBeads(&d) {
		t.Fatal("empty creative board must save")
	}
	d.Finished = true
	if validateBeads(&d) {
		t.Fatal("empty work cannot be finished")
	}
	d.Pixels[0] = 1
	if !validateBeads(&d) {
		t.Fatal("finished creative work rejected")
	}
	d.Mode = "guided"
	d.Target = make([]int, 256)
	d.Target[0] = 2
	if validateBeads(&d) {
		t.Fatal("wrong-color guided work cannot finish")
	}
	d.Pixels[0] = 2
	if !validateBeads(&d) {
		t.Fatal("complete guided work rejected")
	}
	d.Pixels[1] = 3
	if validateBeads(&d) {
		t.Fatal("extra bead overlooked")
	}
	d.Finished = false
	d.Pixels[1] = 33
	if validateBeads(&d) {
		t.Fatal("unknown palette index accepted")
	}
	d.Pixels[1] = 0
	d.Size = 64
	if validateBeads(&d) {
		t.Fatal("oversized board accepted")
	}
}

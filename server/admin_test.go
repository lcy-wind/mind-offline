package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPagination(t *testing.T) {
	for _, v := range []struct {
		query string
		page  int
		ok    bool
	}{{"", 1, true}, {"?page=2", 2, true}, {"?page=0", 0, false}, {"?page=-1", 0, false}, {"?page=abc", 0, false}, {"?page=1000001", 0, false}} {
		w := httptest.NewRecorder()
		p, ok := pagination(w, httptest.NewRequest("GET", "/"+v.query, nil))
		if p != v.page || ok != v.ok {
			t.Fatalf("%s: %d %t", v.query, p, ok)
		}
		if !ok && w.Code != 400 {
			t.Fatal("invalid page must be rejected")
		}
	}
}
func TestSearchLength(t *testing.T) {
	w := httptest.NewRecorder()
	_, ok := searchQuery(w, httptest.NewRequest("GET", "/?q="+strings.Repeat("a", 81), nil))
	if ok || w.Code != 400 {
		t.Fatal("oversized query accepted")
	}
}

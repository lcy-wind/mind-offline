package main

import (
	"strings"
	"testing"
)

func TestCredentials(t *testing.T) {
	for _, v := range []struct {
		name, password string
		valid          bool
	}{{"cole", "secret123", true}, {"ab", "secret123", false}, {"a b", "secret123", false}, {"用户", "secret123", false}, {"cole", "1234567", false}, {"cole", strings.Repeat("a", 73), false}, {"cole", strings.Repeat("字", 25), false}, {"cole_123", "密码测试八个字符", true}} {
		in := credentials{Username: normalizeUsername(v.name), Password: v.password}
		if validCredentials(in) != v.valid {
			t.Errorf("validation mismatch for %q", v.name)
		}
	}
	if normalizeUsername(" COLE ") != "cole" {
		t.Fatal("username normalization")
	}
}

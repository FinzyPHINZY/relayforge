package delivery

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestResponseExcerptFitsDatabaseLimit(t *testing.T) {
	raw := []byte(strings.Repeat("€", 342))
	raw = append(raw, 0, 0xff, '\n')
	got := responseExcerpt(raw)
	if len([]byte(got)) > 1024 || !utf8.ValidString(got) || strings.ContainsRune(got, 0) {
		t.Fatalf("unsafe response excerpt: bytes=%d valid=%t", len(got), utf8.ValidString(got))
	}
}

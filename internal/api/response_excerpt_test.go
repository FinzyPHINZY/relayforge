package api

import "testing"

func TestSafeResponseExcerpt(t *testing.T) {
	for _, item := range []struct {
		input string
		want  string
	}{{"Too Many Requests", "Too Many Requests"}, {"  Service Unavailable  ", "Service Unavailable"}, {`{"token":"secret"}`, "[Response body withheld]"}, {"user@example.test", "[Response body withheld]"}} {
		got := safeResponseExcerpt(&item.input)
		if got == nil || *got != item.want {
			t.Fatalf("input=%q got=%v want=%q", item.input, got, item.want)
		}
	}
}

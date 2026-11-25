package accounts

import (
	"bytes"
	"testing"
)

func TestNormaliseEmail(t *testing.T) {
	got, err := NormaliseEmail("  Bo@Example.COM ")
	if err != nil || got != "bo@example.com" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"bo", "a@b@c", "Bo <bo@x.io>", "", "a@" + string(bytes.Repeat([]byte("x"), 260)) + ".io"} {
		if _, err := NormaliseEmail(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestHashIPStable(t *testing.T) {
	a := &Store{IPHashKey: []byte("key-one")}
	b := &Store{IPHashKey: []byte("key-two")}
	if !bytes.Equal(a.HashIP("1.2.3.4"), a.HashIP("1.2.3.4")) {
		t.Fatal("hash not stable")
	}
	if bytes.Equal(a.HashIP("1.2.3.4"), b.HashIP("1.2.3.4")) {
		t.Fatal("different keys must differ")
	}
	if a.HashIP("") != nil {
		t.Fatal("empty ip must give nil")
	}
}

func TestNormaliseIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4":                  "1.2.3.4",
		"::ffff:1.2.3.4":           "1.2.3.4",
		"2001:db8:1:2:aaaa::1":     "2001:db8:1:2::",
		"2001:db8:1:2:ffff:ffff::": "2001:db8:1:2::",
		"fe80::1%eth0":             "fe80::",
		"":                         "",
		"not-an-ip":                "not-an-ip",
	}
	for in, want := range cases {
		if got := NormaliseIP(in); got != want {
			t.Fatalf("NormaliseIP(%q)=%q want %q", in, got, want)
		}
	}
	if NormaliseIP("2001:db8:1:2::1") == NormaliseIP("2001:db8:1:3::1") {
		t.Fatal("different /64s must not share a subject")
	}
	s := &Store{IPHashKey: []byte("k")}
	if !bytes.Equal(s.HashIP("::ffff:1.2.3.4"), s.HashIP("1.2.3.4")) {
		t.Fatal("IPv4-mapped address must hash like the IPv4 address")
	}
	if !bytes.Equal(s.HashIP("2001:db8:1:2::1"), s.HashIP("2001:db8:1:2:dead:beef::")) {
		t.Fatal("addresses in one /64 must hash equal")
	}
	if bytes.Equal(s.HashIP("2001:db8:1:2::1"), s.HashIP("2001:db8:1:3::1")) {
		t.Fatal("different /64s must hash differently")
	}
}

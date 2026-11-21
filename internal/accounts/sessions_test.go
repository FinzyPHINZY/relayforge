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

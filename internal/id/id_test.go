package id

import "testing"

func TestUUIDv7(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 36 || a[14] != '7' || a[19] < '8' || a == b {
		t.Fatalf("invalid UUIDv7: %q %q", a, b)
	}
}

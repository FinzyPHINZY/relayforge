package api

import (
	"testing"
	"time"
)

func TestClaimKeyTouchOncePerMinute(t *testing.T) {
	var s Server
	t0 := time.Unix(1_700_000_000, 0)
	if !s.claimKeyTouch("a", t0) {
		t.Fatal("first use not claimed")
	}
	for _, at := range []time.Duration{0, time.Second, 59 * time.Second} {
		if s.claimKeyTouch("a", t0.Add(at)) {
			t.Fatalf("claimed again after %v", at)
		}
	}
	if !s.claimKeyTouch("b", t0) {
		t.Fatal("other key throttled")
	}
	if !s.claimKeyTouch("a", t0.Add(time.Minute)) {
		t.Fatal("not claimed after a minute")
	}
	if s.claimKeyTouch("a", t0.Add(time.Minute+time.Second)) {
		t.Fatal("claimed twice in the next minute")
	}
}

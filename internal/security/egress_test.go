package security

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestPinnedClientBlocksPrivateAndRedirectTargets(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{"https://127.0.0.1/", "https://169.254.169.254/", "https://[::1]/", "http://8.8.8.8/", "https://user:pass@8.8.8.8/"} {
		if _, err := PinnedClient(ctx, raw, time.Second, true); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	client, err := PinnedClient(ctx, "https://8.8.8.8/", time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://127.0.0.1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(request, nil); err != http.ErrUseLastResponse {
		t.Fatalf("redirect policy: %v", err)
	}
}

func TestPinnedClientRevalidatesEveryDNSAnswer(t *testing.T) {
	ctx := context.Background()
	lookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	if _, err := pinnedClientWithLookup(ctx, "https://subscriber.example/", time.Second, false, lookup); err != nil {
		t.Fatal(err)
	}
	lookup = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, err := pinnedClientWithLookup(ctx, "https://subscriber.example/", time.Second, false, lookup); err == nil {
		t.Fatal("accepted rebinding to private address")
	}
	lookup = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("::ffff:127.0.0.1")}}, nil
	}
	if _, err := pinnedClientWithLookup(ctx, "https://subscriber.example/", time.Second, false, lookup); err == nil {
		t.Fatal("accepted IPv4-mapped loopback")
	}
}

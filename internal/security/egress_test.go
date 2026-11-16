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
	if _, err := pinnedClientWithLookup(ctx, "https://subscriber.example/", time.Second, false, "", lookup); err != nil {
		t.Fatal(err)
	}
	lookup = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, err := pinnedClientWithLookup(ctx, "https://subscriber.example/", time.Second, false, "", lookup); err == nil {
		t.Fatal("accepted rebinding to private address")
	}
	lookup = func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("::ffff:127.0.0.1")}}, nil
	}
	if _, err := pinnedClientWithLookup(ctx, "https://subscriber.example/", time.Second, false, "", lookup); err == nil {
		t.Fatal("accepted IPv4-mapped loopback")
	}
}

func TestPinnedClientRejectsDNSRebindingBetweenAttempts(t *testing.T) {
	lookups := 0
	lookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		lookups++
		if lookups == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	for attempt := 1; attempt <= 2; attempt++ {
		_, err := pinnedClientWithLookup(context.Background(), "https://subscriber.example/hook", time.Second, false, "", lookup)
		if attempt == 1 && err != nil {
			t.Fatalf("public first resolution rejected: %v", err)
		}
		if attempt == 2 && err == nil {
			t.Fatal("metadata address accepted after DNS rebinding")
		}
	}
	if lookups != 2 {
		t.Fatalf("DNS lookups=%d", lookups)
	}
}

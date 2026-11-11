package relayforge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestPublishRetriesWithSameIdempotencyKey(t *testing.T) {
	var calls int
	var firstKey string
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v1/events" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer whk_test" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		key := r.Header.Get("Idempotency-Key")
		if calls == 1 {
			firstKey = key
			return response(503, ""), nil
		}
		if key == "" || key != firstKey {
			t.Errorf("idempotency key changed across attempts")
		}
		return response(202, `{"id":"event-1","type":"order.created","status":"accepted"}`), nil
	})
	client, err := New("https://relayforge.example", "whk_test", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	event, err := client.Publish(context.Background(), "order.created", map[string]string{"order_id": "1"}, "")
	if err != nil || event.ID != "event-1" || calls != 2 {
		t.Fatalf("event=%+v calls=%d err=%v", event, calls, err)
	}
}

func TestPublishReturnsProblemWithoutRetry(t *testing.T) {
	var calls int
	transport := transportFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return response(409, `{"code":"idempotency_conflict","detail":"Key already used"}`), nil
	})
	client, err := New("https://relayforge.example", "whk_test", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Publish(context.Background(), "order.created", map[string]string{"order_id": "1"}, "same")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "idempotency_conflict" || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

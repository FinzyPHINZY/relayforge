package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"webhook_delivery/internal/security"
)

func TestFixtureVerifiesAndRecordsRetry(t *testing.T) {
	f := &fixture{seen: make(map[string]bool)}
	w := httptest.NewRecorder()
	f.configure(w, httptest.NewRequest(http.MethodPost, "/admin/config", bytes.NewBufferString(`{"secret":"whsec_test","failures":1}`)))
	if w.Code != 204 {
		t.Fatalf("configure=%d", w.Code)
	}
	body := []byte(`{"id":"event"}`)
	ts := time.Now().Unix()
	send := func() int {
		r := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
		r.Header.Set("webhook-id", "delivery-1")
		r.Header.Set("webhook-attempt", "1")
		r.Header.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
		r.Header.Set("webhook-signature", security.WebhookSignature("whsec_test", ts, body))
		w := httptest.NewRecorder()
		f.receive(w, r)
		return w.Code
	}
	if got := send(); got != 503 {
		t.Fatalf("first=%d", got)
	}
	if got := send(); got != 204 {
		t.Fatalf("second=%d", got)
	}
	if len(f.receipts) != 2 || !f.receipts[0].ValidSignature || f.receipts[0].Duplicate || !f.receipts[1].Duplicate {
		t.Fatalf("receipts=%+v", f.receipts)
	}
}

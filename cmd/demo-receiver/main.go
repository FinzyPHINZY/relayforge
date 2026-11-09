package main

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"webhook_delivery/internal/security"
)

type receipt struct {
	WebhookID      string    `json:"webhook_id"`
	Attempt        string    `json:"attempt"`
	ValidSignature bool      `json:"valid_signature"`
	Duplicate      bool      `json:"duplicate"`
	HTTPStatus     int       `json:"http_status"`
	ReceivedAt     time.Time `json:"received_at"`
}
type fixture struct {
	mu       sync.Mutex
	secret   string
	failures int
	receipts []receipt
	seen     map[string]bool
}

func main() {
	f := &fixture{seen: make(map[string]bool)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("POST /admin/config", f.configure)
	mux.HandleFunc("GET /admin/receipts", f.list)
	mux.HandleFunc("POST /webhooks", f.receive)
	log.Fatal((&http.Server{Addr: ":8090", Handler: mux, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 * 1024}).ListenAndServe())
}
func (f *fixture) configure(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Secret   string `json:"secret"`
		Failures int    `json:"failures"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil || input.Secret == "" || len(input.Secret) > 256 || input.Failures < 0 || input.Failures > 8 {
		http.Error(w, "invalid fixture configuration", 400)
		return
	}
	f.mu.Lock()
	f.secret = input.Secret
	f.failures = input.Failures
	f.receipts = nil
	f.seen = make(map[string]bool)
	f.mu.Unlock()
	w.WriteHeader(204)
}
func (f *fixture) list(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	items := append([]receipt(nil), f.receipts...)
	remaining := f.failures
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "failures_remaining": remaining})
}
func (f *fixture) receive(w http.ResponseWriter, r *http.Request) {
	const maxBody = 300 * 1024
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "body too large", 413)
		return
	}
	id := r.Header.Get("webhook-id")
	ts, err := strconv.ParseInt(r.Header.Get("webhook-timestamp"), 10, 64)
	validTime := err == nil && ts >= time.Now().Add(-5*time.Minute).Unix() && ts <= time.Now().Add(5*time.Minute).Unix()
	f.mu.Lock()
	valid := validTime && id != "" && f.secret != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("webhook-signature")), []byte(security.WebhookSignature(f.secret, ts, body))) == 1
	status := 401
	if valid {
		status = 204
		if f.failures > 0 {
			status = 503
			f.failures--
		}
	}
	duplicate := f.seen[id]
	if id != "" {
		f.seen[id] = true
	}
	f.receipts = append(f.receipts, receipt{WebhookID: id, Attempt: r.Header.Get("webhook-attempt"), ValidSignature: valid, Duplicate: duplicate, HTTPStatus: status, ReceivedAt: time.Now().UTC()})
	if len(f.receipts) > 100 {
		f.receipts = f.receipts[len(f.receipts)-100:]
	}
	f.mu.Unlock()
	w.WriteHeader(status)
}

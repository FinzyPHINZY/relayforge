package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/security"
)

type Result struct {
	Outcome, ErrorCode, ResponseExcerpt string
	HTTPStatus                          int
	Retryable                           bool
	RetryAfter                          time.Duration
	Duration                            time.Duration
}

type Sender struct {
	DB              *pgxpool.Pool
	EncryptionKey   []byte
	EncryptionKeyID string
	AllowHTTPTest   bool
}

func (s Sender) Send(ctx context.Context, c Claimed) Result {
	started := time.Now()
	result := Result{Outcome: "unknown"}
	client, err := security.PinnedClient(ctx, c.DestinationURL, c.Timeout, s.AllowHTTPTest)
	if err != nil {
		return Result{Outcome: "security_blocked", ErrorCode: "destination_blocked", Duration: time.Since(started)}
	}
	var secretID, keyID, secret string
	var ciphertext, nonce []byte
	err = s.DB.QueryRow(ctx, `SELECT id,key_id,ciphertext,nonce FROM endpoint_secrets WHERE endpoint_id=$1 AND organization_id=$2 AND state='active'`, c.EndpointID, c.OrganizationID).Scan(&secretID, &keyID, &ciphertext, &nonce)
	if err != nil || keyID != s.EncryptionKeyID {
		return Result{Outcome: "network_error", ErrorCode: "secret_unavailable", Retryable: true, Duration: time.Since(started)}
	}
	secret, err = security.Decrypt(s.EncryptionKey, ciphertext, nonce, c.EndpointID)
	if err != nil {
		return Result{Outcome: "network_error", ErrorCode: "secret_unavailable", Retryable: true, Duration: time.Since(started)}
	}
	// Persist the selected secret before sending so an abandoned attempt retains
	// its signing metadata if the process dies after the receiver accepts.
	tag, err := s.DB.Exec(ctx, `UPDATE delivery_attempts a SET signing_secret_id=$2 FROM deliveries d WHERE a.id=$1 AND a.delivery_id=d.id AND d.lease_owner=$3 AND d.lease_generation=$4 AND d.status='processing' AND d.lease_expires_at>now()`, c.AttemptID, secretID, c.WorkerID, c.LeaseGeneration)
	if err != nil || tag.RowsAffected() != 1 {
		return Result{Outcome: "unknown", ErrorCode: "stale_lease", Duration: time.Since(started)}
	}
	body, err := json.Marshal(struct {
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		CreatedAt string          `json:"created_at"`
		Data      json.RawMessage `json:"data"`
	}{c.EventID, c.EventType, c.EventCreatedAt.UTC().Format(time.RFC3339Nano), json.RawMessage(c.Payload)})
	if err != nil {
		return Result{Outcome: "network_error", ErrorCode: "serialization_error", Retryable: true, Duration: time.Since(started)}
	}
	timestamp := time.Now().Unix()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.DestinationURL, bytes.NewReader(body))
	if err != nil {
		return Result{Outcome: "security_blocked", ErrorCode: "invalid_destination", Duration: time.Since(started)}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("webhook-id", c.ID)
	request.Header.Set("webhook-timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("webhook-signature", security.WebhookSignature(secret, timestamp, body))
	request.Header.Set("webhook-attempt", strconv.Itoa(c.AttemptNumber))
	request.Header.Set("User-Agent", "ReliableWebhookPlatform/1.0")
	response, err := client.Do(request)
	if err != nil {
		code := "network_error"
		outcome := "network_error"
		var netErr interface{ Timeout() bool }
		if errors.As(err, &netErr) && netErr.Timeout() {
			code = "timeout"
			outcome = "timeout"
		}
		return Result{Outcome: outcome, ErrorCode: code, Retryable: true, Duration: time.Since(started)}
	}
	defer response.Body.Close()
	excerptBytes, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	excerpt := responseExcerpt(excerptBytes)
	result = Result{Outcome: "http_error", HTTPStatus: response.StatusCode, ResponseExcerpt: excerpt, Duration: time.Since(started)}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		result.Outcome = "success"
		return result
	}
	status := response.StatusCode
	result.Retryable = status == 408 || status == 409 || status == 425 || status == 429 || status >= 500
	if status == 429 || status == 503 {
		result.RetryAfter = RetryAfter(response.Header.Get("Retry-After"), time.Now())
	}
	result.ErrorCode = fmt.Sprintf("http_%d", status)
	return result
}

func responseExcerpt(raw []byte) string {
	// Keep only printable UTF-8 and count encoded bytes, matching the database
	// limit even when a response ends in the middle of a multibyte character.
	out := make([]byte, 0, len(raw))
	for _, r := range string(raw) {
		if r == '\ufffd' || (r < 32 && r != '\n' && r != '\t') || r == 127 {
			continue
		}
		encoded := []byte(string(r))
		if len(out)+len(encoded) > 1024 {
			break
		}
		out = append(out, encoded...)
	}
	return string(out)
}

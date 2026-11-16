package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/security"
)

// This helper runs in a separate OS process. The first invocation deliberately
// stops after the receiver acknowledges the HTTP request, before Complete.
func TestV01WorkerProcess(t *testing.T) {
	mode := os.Getenv("RELAYFORGE_V01_PROCESS")
	if mode == "" {
		return
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("RELAYFORGE_V01_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if mode == "retry" {
		count, err := delivery.ReclaimExpired(ctx, pool, 10)
		if err != nil || count != 1 {
			t.Fatalf("reclaim count=%d err=%v", count, err)
		}
	}
	claim, err := delivery.ClaimOne(ctx, pool, "v01-"+mode, 5*time.Second)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	result := testSender(pool).Send(ctx, *claim)
	if result.Outcome != "success" {
		t.Fatalf("send result=%+v", result)
	}
	if mode == "crash" {
		fmt.Printf("SENT %s\n", claim.ID)
		for {
			time.Sleep(time.Hour)
		} // parent sends SIGKILL; Complete is unreachable.
	}
	applied, err := delivery.Complete(ctx, pool, *claim, result)
	if err != nil || !applied {
		t.Fatalf("complete applied=%v err=%v", applied, err)
	}
	fmt.Printf("DONE %s\n", claim.ID)
}

func TestRealProcessCrashAfterReceiverAcknowledges(t *testing.T) {
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "real process crash")
	type receipt struct {
		id, attempt string
		valid       bool
	}
	var mu sync.Mutex
	var receipts []receipt
	secret := "whsec_test_delivery_secret"
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		ts, err := strconv.ParseInt(r.Header.Get("webhook-timestamp"), 10, 64)
		valid := err == nil && security.WebhookSignature(secret, ts, body.Bytes()) == r.Header.Get("webhook-signature")
		mu.Lock()
		receipts = append(receipts, receipt{id: r.Header.Get("webhook-id"), attempt: r.Header.Get("webhook-attempt"), valid: valid})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	_, endpointSecret := addDeliveryEndpoint(t, pool, org, receiver.URL)
	if endpointSecret != secret {
		t.Fatal("unexpected test signing secret")
	}
	eventID := publishDeliveryEvent(t, apiServer, key)
	if count, err := delivery.DispatchBatch(context.Background(), pool, 10); err != nil || count != 1 {
		t.Fatalf("dispatch count=%d err=%v", count, err)
	}

	first := startV01Process(t, pool, "crash")
	line := waitV01Line(t, first.lines, 10*time.Second)
	if !strings.HasPrefix(line, "SENT ") {
		t.Fatalf("first process: %q; stderr=%s", line, first.stderr.String())
	}
	deliveryID := strings.TrimPrefix(line, "SENT ")
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := first.cmd.Wait(); err == nil {
		t.Fatal("crash process exited without SIGKILL")
	}
	var status string
	var attempts int
	if err := pool.QueryRow(context.Background(), `SELECT status,attempt_count FROM deliveries WHERE id=$1 AND event_id=$2`, deliveryID, eventID).Scan(&status, &attempts); err != nil || status != "processing" || attempts != 1 {
		t.Fatalf("before reclaim status=%s attempts=%d err=%v", status, attempts, err)
	}
	time.Sleep(5200 * time.Millisecond) // allow the real lease to expire
	second := startV01Process(t, pool, "retry")
	line = waitV01Line(t, second.lines, 10*time.Second)
	if err := second.cmd.Wait(); err != nil {
		t.Fatalf("retry process: %v; stderr=%s", err, second.stderr.String())
	}
	if line != "DONE "+deliveryID {
		t.Fatalf("retry process: %q", line)
	}

	mu.Lock()
	got := append([]receipt(nil), receipts...)
	mu.Unlock()
	if len(got) != 2 || got[0].id != deliveryID || got[1].id != deliveryID || got[0].attempt != "1" || got[1].attempt != "2" || !got[0].valid || !got[1].valid {
		t.Fatalf("receiver receipts=%+v", got)
	}
	var firstOutcome, firstError, secondOutcome, finalStatus, outboxStatus string
	if err := pool.QueryRow(context.Background(), `SELECT a.outcome,a.error_code,b.outcome,d.status,o.status FROM delivery_attempts a JOIN delivery_attempts b ON b.delivery_id=a.delivery_id AND b.attempt_number=2 JOIN deliveries d ON d.id=a.delivery_id JOIN outbox o ON o.event_id=d.event_id WHERE a.delivery_id=$1 AND a.attempt_number=1`, deliveryID).Scan(&firstOutcome, &firstError, &secondOutcome, &finalStatus, &outboxStatus); err != nil {
		t.Fatal(err)
	}
	if firstOutcome != "unknown" || firstError != "lease_expired" || secondOutcome != "success" || finalStatus != "succeeded" || outboxStatus != "dispatched" {
		t.Fatalf("history=%s/%s then %s; delivery=%s outbox=%s", firstOutcome, firstError, secondOutcome, finalStatus, outboxStatus)
	}
	var events, deliveries int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM events WHERE id=$1),(SELECT count(*) FROM deliveries WHERE event_id=$1)`, eventID).Scan(&events, &deliveries); err != nil || events != 1 || deliveries != 1 {
		t.Fatalf("durability events=%d deliveries=%d err=%v", events, deliveries, err)
	}
}

type v01Process struct {
	cmd    *exec.Cmd
	lines  chan string
	stderr bytes.Buffer
}

func startV01Process(t *testing.T, pool *pgxpool.Pool, mode string) *v01Process {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := &v01Process{cmd: exec.Command(path, "-test.run=^TestV01WorkerProcess$"), lines: make(chan string, 2)}
	p.cmd.Env = append(os.Environ(), "RELAYFORGE_V01_PROCESS="+mode, "RELAYFORGE_V01_DATABASE_URL="+pool.Config().ConnString())
	p.cmd.Stderr = &p.stderr
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p.cmd.ProcessState == nil {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
		}
	})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			p.lines <- scanner.Text()
		}
		close(p.lines)
	}()
	return p
}

func waitV01Line(t *testing.T, lines <-chan string, timeout time.Duration) string {
	t.Helper()
	select {
	case line, ok := <-lines:
		if !ok {
			t.Fatal("worker process exited without milestone")
		}
		return line
	case <-time.After(timeout):
		t.Fatal("worker process milestone timed out")
		return ""
	}
}

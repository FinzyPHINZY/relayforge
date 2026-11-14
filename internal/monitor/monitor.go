package monitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// Monitor only notifies when the failure set changes or recovers. A failed
// notification is retried on the next tick rather than marked delivered.
type Monitor struct {
	Check  func(context.Context) []string
	Notify func(context.Context, string) error
	Last   string
}

func (m *Monitor) Tick(ctx context.Context) error {
	if m.Check == nil || m.Notify == nil {
		return errors.New("monitor is not configured")
	}
	reasons := m.Check(ctx)
	slices.Sort(reasons)
	current := strings.Join(reasons, ", ")
	if current == m.Last {
		return nil
	}
	if current == "" && m.Last == "" {
		return nil
	}
	message := "RelayForge recovered: all checks healthy"
	if current != "" {
		message = fmt.Sprintf("RelayForge needs attention: %s", current)
	}
	if err := m.Notify(ctx, message); err != nil {
		return err
	}
	m.Last = current
	return nil
}

func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	check := func() {
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := m.Tick(checkCtx); err != nil {
			slog.Warn("monitor notification failed; will retry")
		}
	}
	check()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

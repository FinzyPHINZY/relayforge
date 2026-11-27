package mail

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

const maxSent = 100

// Message is an outgoing email; HTML is optional and never logged.
type Message struct{ To, Subject, Text, HTML string }

type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// LogMailer logs only the subject and the recipient domain, never the address or body.
type LogMailer struct {
	Logger *slog.Logger
	// LogBody logs the full message text; for local development only.
	LogBody bool
	Sent    []Message
	mu      sync.Mutex
}

func (l *LogMailer) Send(ctx context.Context, m Message) error {
	l.mu.Lock()
	l.Sent = append(l.Sent, m)
	if len(l.Sent) > maxSent {
		l.Sent = append([]Message(nil), l.Sent[len(l.Sent)-maxSent:]...)
	}
	l.mu.Unlock()
	if l.Logger != nil && l.LogBody {
		l.Logger.InfoContext(ctx, "mail sent (log mailer)", "subject", m.Subject, "to", m.To, "text", m.Text)
	} else if l.Logger != nil {
		l.Logger.InfoContext(ctx, "mail sent (log mailer)", "subject", m.Subject, "recipient_domain", recipientDomain(m.To))
	}
	return nil
}

func (l *LogMailer) Messages() []Message {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Message(nil), l.Sent...)
}

func recipientDomain(to string) string {
	if i := strings.LastIndex(to, "@"); i >= 0 {
		return to[i+1:]
	}
	return ""
}

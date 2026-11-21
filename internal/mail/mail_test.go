package mail

import (
	"strings"
	"testing"
)

func TestSignInMessage(t *testing.T) {
	link := "https://app.relayforge.site/auth/email?token=rfm_abc"
	m := SignInMessage("bo@example.com", link)
	if m.Subject != "Your RelayForge sign-in link" {
		t.Fatalf("subject %q", m.Subject)
	}
	if m.To != "bo@example.com" {
		t.Fatalf("to %q", m.To)
	}
	if !strings.Contains(m.Text, link) || !strings.Contains(m.Text, "15 minutes") || !strings.Contains(m.Text, "If you didn't ask for this, ignore this email.") {
		t.Fatalf("text %q", m.Text)
	}
	if strings.Contains(m.Text, "bo@example.com") {
		t.Fatal("text contains raw recipient")
	}
}

func TestBuildSendEmailInput(t *testing.T) {
	in := buildSendEmailInput("RelayForge <sign-in@relayforge.site>", Message{To: "a@b.co", Subject: "s", Text: "t"})
	if *in.FromEmailAddress != "RelayForge <sign-in@relayforge.site>" || in.Destination.ToAddresses[0] != "a@b.co" {
		t.Fatalf("%+v", in)
	}
	if in.Content.Simple == nil || *in.Content.Simple.Body.Text.Data != "t" || in.Content.Simple.Body.Html != nil {
		t.Fatalf("body %+v", in.Content.Simple)
	}
}

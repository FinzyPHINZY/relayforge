package mail

import (
	"strings"
	"testing"
)

func TestSignInHTML(t *testing.T) {
	link := "https://app.relayforge.site/auth/email?token=rfm_abc"
	m := SignInMessage("bo@example.com", link)
	if m.HTML == "" {
		t.Fatal("html part is empty")
	}
	if n := strings.Count(m.HTML, link); n != 2 {
		t.Fatalf("link appears %d times, want 2 (href + fallback)", n)
	}
	if n := strings.Count(m.HTML, `href="`+link+`"`); n != 1 {
		t.Fatalf("link appears %d times in href, want 1", n)
	}
	for _, want := range []string{
		"15 minutes",
		"Your secure sign-in link — expires in 15 minutes.",
		"Sign in to RelayForge",
		"This link expires in 15 minutes and works once.",
		"Didn't ask for this? You can ignore this email — no one can sign in without it.",
		"RelayForge · relayforge.site",
		`lang="en"`,
		`<meta name="color-scheme" content="light only">`,
		`role="presentation"`,
	} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("html missing %q", want)
		}
	}
	if strings.Contains(m.HTML, "bo@example.com") || strings.Contains(m.Text, "bo@example.com") {
		t.Fatal("message body contains the recipient address")
	}
	if !strings.Contains(m.Text, link) || !strings.Contains(m.Text, "15 minutes") {
		t.Fatalf("text %q", m.Text)
	}
}

func TestSignInHTMLEscapesLink(t *testing.T) {
	m := SignInMessage("bo@example.com", `https://x.test/?t="><script>alert(1)</script>`)
	if strings.Contains(m.HTML, "<script>") {
		t.Fatal("html contains an unescaped script tag")
	}
}

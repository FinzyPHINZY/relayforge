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
		"Your secure sign-in link. Expires in 15 minutes.",
		"Sign in to RelayForge",
		"This link expires in 15 minutes and works once.",
		"Click below to open your console. No password needed.",
		"Didn't ask for this? You can ignore this email. No one can sign in without it.",
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

func TestSignInHasNoDashes(t *testing.T) {
	m := SignInMessage("bo@example.com", "https://app.relayforge.site/auth/email?token=rfm_abc")
	for _, d := range []string{"\u2014", "\u2013"} {
		if strings.Contains(m.HTML, d) || strings.Contains(m.Text, d) || strings.Contains(m.Subject, d) {
			t.Fatalf("message contains dash %q", d)
		}
	}
}

func TestSignInHTMLEscapesLink(t *testing.T) {
	m := SignInMessage("bo@example.com", `https://x.test/?t="><script>alert(1)</script>`)
	if strings.Contains(m.HTML, "<script>") {
		t.Fatal("html contains an unescaped script tag")
	}
	if !strings.Contains(m.HTML, `href="https://x.test/?t=%22%3e%3cscript%3ealert%281%29%3c/script%3e"`) {
		t.Fatal("hostile link quote not encoded in href")
	}
	js := SignInMessage("bo@example.com", "javascript:alert(1)")
	if strings.Contains(js.HTML, `href="javascript:`) {
		t.Fatal("javascript: url rendered in href")
	}
	if !strings.Contains(js.HTML, `href="#ZgotmplZ"`) {
		t.Fatal("javascript: url was not neutralised to #ZgotmplZ")
	}
}

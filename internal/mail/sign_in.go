package mail

import (
	"bytes"
	_ "embed"
	"html/template"
	"strconv"
	"time"
)

const signInExpiresMinutes = 15

//go:embed templates/sign_in.html
var signInHTMLSource string

// html/template strips comments, so outlook conditional wrappers are injected as trusted constants.
var signInHTML = template.Must(template.New("sign_in").Funcs(template.FuncMap{
	"msoOpen": func() template.HTML {
		return `<!--[if mso]><table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" align="center"><tr><td><![endif]-->`
	},
	"msoClose": func() template.HTML { return `<!--[if mso]></td></tr></table><![endif]-->` },
}).Parse(signInHTMLSource))

type signInData struct {
	Link           string
	ExpiresMinutes int
	Year           int
}

func SignInMessage(to, link string) Message {
	text := "Use this link to sign in to RelayForge:\n\n" + link +
		"\n\nThe link expires in " + strconv.Itoa(signInExpiresMinutes) + " minutes and can be used once.\n\nIf you didn't ask for this, ignore this email.\n"
	m := Message{To: to, Subject: "Your RelayForge sign-in link", Text: text}
	var buf bytes.Buffer
	// on a render error the text part alone is still a usable email
	if err := signInHTML.Execute(&buf, signInData{Link: link, ExpiresMinutes: signInExpiresMinutes, Year: time.Now().UTC().Year()}); err == nil {
		m.HTML = buf.String()
	}
	return m
}

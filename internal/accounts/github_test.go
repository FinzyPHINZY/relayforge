package accounts

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGitHubStartURL(t *testing.T) {
	g := GitHubConfig{ClientID: "cid", RedirectURL: "https://app.example/auth/github/callback"}
	st, err := g.Start()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(st.AuthorizeURL)
	if err != nil || u.Host != "github.com" || u.Path != "/login/oauth/authorize" {
		t.Fatalf("url %q %v", st.AuthorizeURL, err)
	}
	q := u.Query()
	sum := sha256.Sum256([]byte(st.Verifier))
	if q.Get("client_id") != "cid" || q.Get("redirect_uri") != g.RedirectURL || q.Get("scope") != "read:user user:email" ||
		q.Get("state") != st.State || q.Get("code_challenge_method") != "S256" ||
		q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Fatalf("bad query %v", q)
	}
	if len(st.State) != 43 {
		t.Fatalf("state len %d", len(st.State))
	}
}

func TestGitHubExchange(t *testing.T) {
	failToken := false
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Method != "POST" || r.Header.Get("Accept") != "application/json" || r.PostForm.Get("code") != "c1" ||
			r.PostForm.Get("code_verifier") != "v1" || r.PostForm.Get("client_secret") != "sec" || r.PostForm.Get("redirect_uri") != "https://r" {
			t.Errorf("bad token request %v", r.PostForm)
		}
		if failToken {
			_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"tok123"}`))
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer tok123" || r.Header.Get("Accept") != "application/vnd.github+json" {
			http.Error(w, "no", 401)
			return false
		}
		return true
	}
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(`{"id":42,"login":"bo","name":"Bo A","avatar_url":"https://a/1"}`))
		}
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(`[{"email":"x@x.io","primary":false,"verified":true},{"email":"Bo@X.io","primary":true,"verified":true}]`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	g := GitHubConfig{ClientID: "cid", ClientSecret: "sec", RedirectURL: "https://r", AuthBase: srv.URL, APIBase: srv.URL}
	p, err := g.Exchange(context.Background(), "c1", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 42 || p.Login != "bo" || p.Name != "Bo A" || p.AvatarURL != "https://a/1" || p.VerifiedPrimaryEmail != "Bo@X.io" {
		t.Fatalf("profile %+v", p)
	}
	failToken = true
	if _, err := g.Exchange(context.Background(), "c1", "v1"); err == nil {
		t.Fatal("expected error on token error")
	}
}

package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
)

const maxGitHubBody = 1 << 20

// ErrGitHubEmailUnverified: no verified primary email, or the address belongs to an unverified account.
var ErrGitHubEmailUnverified = errors.New("github_email_unverified")

// ErrUserInactive reports a user that is deleting or deleted.
var ErrUserInactive = errors.New("user is not active")

type GitHubConfig struct {
	ClientID, ClientSecret, RedirectURL string
	AuthBase, APIBase                   string // default https://github.com and https://api.github.com
	HTTP                                *http.Client
}

type OAuthStart struct{ AuthorizeURL, State, Verifier string }

type GitHubProfile struct {
	ID                                           int64
	Login, Name, AvatarURL, VerifiedPrimaryEmail string
}

func (g GitHubConfig) authBase() string {
	if g.AuthBase != "" {
		return strings.TrimRight(g.AuthBase, "/")
	}
	return "https://github.com"
}

func (g GitHubConfig) apiBase() string {
	if g.APIBase != "" {
		return strings.TrimRight(g.APIBase, "/")
	}
	return "https://api.github.com"
}

func (g GitHubConfig) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func randomB64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Start builds the authorize URL with a fresh state and PKCE S256 challenge.
func (g GitHubConfig) Start() (OAuthStart, error) {
	state, err := randomB64(32)
	if err != nil {
		return OAuthStart{}, err
	}
	verifier, err := randomB64(32)
	if err != nil {
		return OAuthStart{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{}
	q.Set("client_id", g.ClientID)
	q.Set("redirect_uri", g.RedirectURL)
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("code_challenge_method", "S256")
	return OAuthStart{AuthorizeURL: g.authBase() + "/login/oauth/authorize?" + q.Encode(), State: state, Verifier: verifier}, nil
}

func (g GitHubConfig) doJSON(req *http.Request, out any) error {
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubBody))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("github: %s returned status %d", req.URL.Path, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// Exchange trades the code for a token and loads the profile. The token is
// never logged or returned.
func (g GitHubConfig) Exchange(ctx context.Context, code, verifier string) (GitHubProfile, error) {
	form := url.Values{}
	form.Set("client_id", g.ClientID)
	form.Set("client_secret", g.ClientSecret)
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", g.RedirectURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.authBase()+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return GitHubProfile{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := g.doJSON(req, &tok); err != nil {
		return GitHubProfile{}, err
	}
	if tok.Error != "" {
		return GitHubProfile{}, fmt.Errorf("github: token exchange failed: %s", tok.Error)
	}
	if tok.AccessToken == "" {
		return GitHubProfile{}, errors.New("github: token exchange returned no access token")
	}
	get := func(path string, out any) error {
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBase()+path, nil)
		if err != nil {
			return err
		}
		r.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		r.Header.Set("Accept", "application/vnd.github+json")
		return g.doJSON(r, out)
	}
	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := get("/user", &u); err != nil {
		return GitHubProfile{}, err
	}
	if u.ID == 0 {
		return GitHubProfile{}, errors.New("github: profile has no id")
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := get("/user/emails", &emails); err != nil {
		return GitHubProfile{}, err
	}
	p := GitHubProfile{ID: u.ID, Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL}
	for _, e := range emails {
		if e.Primary && e.Verified {
			p.VerifiedPrimaryEmail = e.Email
			break
		}
	}
	return p, nil
}

// SignInWithGitHub links or creates the user for a GitHub profile, opens a
// session and audits the sign-in in each of the user's active orgs. Identity
// first, then verified-email match, then create.
func (s *Store) SignInWithGitHub(ctx context.Context, p GitHubProfile, ip, userAgent string) (string, Session, User, error) {
	if p.ID == 0 {
		return "", Session{}, User{}, errors.New("accounts: github profile id required")
	}
	subject := strconv.FormatInt(p.ID, 10)
	display := p.Name
	if display == "" {
		display = p.Login
	}
	display = truncateRunes(display, 120)
	p.AvatarURL = truncateRunes(p.AvatarURL, 2048)
	now := s.now()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", Session{}, User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var uid string
	err = tx.QueryRow(ctx, `SELECT user_id FROM user_identities WHERE provider='github' AND provider_subject=$1`, subject).Scan(&uid)
	switch {
	case err == nil:
		// rule 1: known identity
	case errors.Is(err, pgx.ErrNoRows):
		if p.VerifiedPrimaryEmail == "" {
			return "", Session{}, User{}, ErrGitHubEmailUnverified
		}
		email, nerr := NormaliseEmail(p.VerifiedPrimaryEmail)
		if nerr != nil {
			return "", Session{}, User{}, ErrGitHubEmailUnverified
		}
		newID, err := id.New()
		if err != nil {
			return "", Session{}, User{}, err
		}
		// Insert verified if absent; the lock below makes concurrent first sign-ins converge.
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, display_name, kind, email_verified_at, created_at) VALUES ($1,$2,$3,'regular',$4,$4)
			ON CONFLICT (email) DO NOTHING`, newID, email, display, now); err != nil {
			return "", Session{}, User{}, err
		}
		var verifiedAt *time.Time
		err = tx.QueryRow(ctx, `SELECT id, email_verified_at FROM users WHERE email=$1 FOR UPDATE`, email).Scan(&uid, &verifiedAt)
		if err != nil {
			return "", Session{}, User{}, err
		}
		if verifiedAt == nil {
			return "", Session{}, User{}, ErrGitHubEmailUnverified // never link or take over an unverified account
		}
	default:
		return "", Session{}, User{}, err
	}

	u := User{ID: uid}
	var dn, av *string
	err = tx.QueryRow(ctx, `UPDATE users SET display_name=$2, avatar_url=$3 WHERE id=$1 AND status='active'
		RETURNING email, display_name, avatar_url, kind, status, email_verified_at IS NOT NULL`,
		uid, display, nullIfEmpty(p.AvatarURL)).Scan(&u.Email, &dn, &av, &u.Kind, &u.Status, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Session{}, User{}, ErrUserInactive
	}
	if err != nil {
		return "", Session{}, User{}, err
	}
	if dn != nil {
		u.DisplayName = *dn
	}
	if av != nil {
		u.AvatarURL = *av
	}
	if err := linkIdentity(ctx, tx, uid, "github", subject, now); err != nil {
		return "", Session{}, User{}, err
	}
	token, sess, err := s.CreateSession(ctx, tx, uid, ip, userAgent)
	if err != nil {
		return "", Session{}, User{}, err
	}
	if err := auditSignIn(ctx, tx, uid, "github"); err != nil {
		return "", Session{}, User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", Session{}, User{}, err
	}
	return token, sess, u, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

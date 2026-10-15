// Package auth signs people in: as a guest (one request, no data asked) or
// with a Google account, and keeps them signed in with a session cookie.
//
//	GET  /api/me                      the current user (or null) and what sign-in methods exist
//	POST /api/auth/guest              start a guest session
//	GET  /api/auth/google/login       redirect to Google's consent page
//	GET  /api/auth/google/callback    where Google sends the person back
//	POST /api/auth/logout             end the session
//	PUT  /api/me/settings             save the settings page's JSON
//
// The cookie holds a random token; the database holds only its SHA-256, so
// a leaked copy of the database signs nobody in. The cookie is HttpOnly
// (scripts cannot read it) and SameSite=Lax (other sites cannot make the
// browser send it with a POST), which is the protection against forged
// requests here.
//
// A guest who then signs in with Google is upgraded in place, so the
// games they already analysed stay theirs; if that Google account was
// already known, the guest's games move to it.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dyanis-chess/backend/internal/store"
)

// Store is the part of the database this package needs; *store.Store in
// production.
type Store interface {
	CreateUser(ctx context.Context, u *store.User) error
	UserByGoogleSub(ctx context.Context, sub string) (*store.User, error)
	LinkGoogle(ctx context.Context, id, sub, name, email, avatarURL string) error
	MergeUsers(ctx context.Context, from, to string) error
	SetSettings(ctx context.Context, id string, settings []byte) error
	CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error
	SessionUser(ctx context.Context, tokenHash string) (*store.User, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

// Google holds the OAuth client from the Google Cloud console. Without a
// ClientID the Google button is hidden and only guests can sign in.
type Google struct {
	ClientID     string
	ClientSecret string
	// RedirectURL must be listed in the console exactly, e.g.
	// http://localhost:8080/api/auth/google/callback
	RedirectURL string

	// The endpoints are Google's; tests point them at a fake.
	AuthURL     string
	TokenURL    string
	UserInfoURL string
}

// Config of a Service.
type Config struct {
	Store        Store
	Google       Google
	CookieSecure bool          // set the Secure flag (true behind https)
	SessionTTL   time.Duration // default 30 days
	HTTPClient   *http.Client  // for Google; default has a 10 s timeout
}

// Service handles sign-in.
type Service struct {
	cfg    Config
	guests *limiter
}

const (
	sessionCookie = "dyanis_session"
	stateCookie   = "dyanis_oauth_state"
	maxSettings   = 8 << 10
)

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 30 * 24 * time.Hour
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	g := &cfg.Google
	if g.AuthURL == "" {
		g.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if g.TokenURL == "" {
		g.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if g.UserInfoURL == "" {
		g.UserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	}
	return &Service{cfg: cfg, guests: newLimiter(30, time.Hour)}
}

func (s *Service) googleEnabled() bool {
	g := s.cfg.Google
	return g.ClientID != "" && g.ClientSecret != "" && g.RedirectURL != ""
}

// Handler returns the routes of this package.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("PUT /api/me/settings", s.putSettings)
	mux.HandleFunc("POST /api/auth/guest", s.guest)
	mux.HandleFunc("GET /api/auth/google/login", s.googleLogin)
	mux.HandleFunc("GET /api/auth/google/callback", s.googleCallback)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	return mux
}

// UserFromRequest returns the signed-in user, or nil.
func (s *Service) UserFromRequest(r *http.Request) *store.User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	u, err := s.cfg.Store.SessionUser(r.Context(), hashToken(c.Value))
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("auth: session lookup: %v", err)
		}
		return nil
	}
	return u
}

// UserID is UserFromRequest reduced to the id ("" = nobody); it is what the
// API package needs to attach a review to its owner.
func (s *Service) UserID(r *http.Request) string {
	if u := s.UserFromRequest(r); u != nil {
		return u.ID
	}
	return ""
}

// ---------------------------------------------------------------- JSON

type userJSON struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Email     string          `json:"email,omitempty"`
	AvatarURL string          `json:"avatarUrl,omitempty"`
	Settings  json.RawMessage `json:"settings,omitempty"`
}

func toJSON(u *store.User) *userJSON {
	if u == nil {
		return nil
	}
	return &userJSON{ID: u.ID, Kind: u.Kind, Name: u.Name, Email: u.Email, AvatarURL: u.AvatarURL, Settings: u.Settings}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ------------------------------------------------------------- handlers

func (s *Service) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"user":   toJSON(s.UserFromRequest(r)),
		"google": s.googleEnabled(),
	})
}

func (s *Service) guest(w http.ResponseWriter, r *http.Request) {
	// A person who is already signed in keeps their account.
	if u := s.UserFromRequest(r); u != nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": toJSON(u)})
		return
	}
	if !s.guests.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "слишком много новых гостей с вашего адреса, попробуйте позже")
		return
	}
	u := &store.User{ID: randomID(), Kind: store.KindGuest, Name: guestName()}
	if err := s.cfg.Store.CreateUser(r.Context(), u); err != nil {
		log.Printf("auth: create guest: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось создать гостя")
		return
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		log.Printf("auth: start session: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось начать сессию")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": toJSON(u)})
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.cfg.Store.DeleteSession(r.Context(), hashToken(c.Value)); err != nil {
			log.Printf("auth: delete session: %v", err)
		}
	}
	s.setCookie(w, r, sessionCookie, "", "/", -1)
	writeJSON(w, http.StatusOK, map[string]any{"user": nil})
}

func (s *Service) putSettings(w http.ResponseWriter, r *http.Request) {
	u := s.UserFromRequest(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "нужно войти")
		return
	}
	// A cross-site form cannot send this content type without a preflight.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "ожидался JSON")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSettings))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "настройки слишком большие")
		return
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON-объект")
		return
	}
	if err := s.cfg.Store.SetSettings(r.Context(), u.ID, body); err != nil {
		log.Printf("auth: save settings: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось сохранить настройки")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- Google

func (s *Service) googleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.googleEnabled() {
		writeError(w, http.StatusServiceUnavailable, "вход через Google не настроен")
		return
	}
	state := randomToken()
	// Lax so that the cookie comes back with Google's redirect (a top-level GET).
	s.setCookie(w, r, stateCookie, state, "/api/auth/google", 10*60)
	q := url.Values{
		"client_id":     {s.cfg.Google.ClientID},
		"redirect_uri":  {s.cfg.Google.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid profile email"},
		"state":         {state},
		"prompt":        {"select_account"},
	}
	http.Redirect(w, r, s.cfg.Google.AuthURL+"?"+q.Encode(), http.StatusFound)
}

func (s *Service) googleCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(code string) {
		http.Redirect(w, r, "/?auth_error="+code+"#account", http.StatusFound)
	}
	if !s.googleEnabled() {
		fail("not_configured")
		return
	}
	// The state cookie is single-use.
	c, _ := r.Cookie(stateCookie)
	s.setCookie(w, r, stateCookie, "", "/api/auth/google", -1)
	state := r.URL.Query().Get("state")
	if c == nil || c.Value == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		fail("state")
		return
	}
	if r.URL.Query().Get("error") != "" { // the person pressed "Cancel"
		fail("denied")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		fail("denied")
		return
	}
	prof, err := s.fetchProfile(r.Context(), code)
	if err != nil {
		log.Printf("auth: google: %v", err)
		fail("google")
		return
	}
	userID, err := s.signInGoogle(r, prof)
	if err != nil {
		log.Printf("auth: google sign-in: %v", err)
		fail("server")
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		log.Printf("auth: start session: %v", err)
		fail("server")
		return
	}
	http.Redirect(w, r, "/#account", http.StatusFound)
}

type profile struct {
	Sub     string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
}

// fetchProfile trades the one-time code for an access token and asks Google
// who it belongs to. Both calls go straight to Google over TLS, so their
// answers need no signature check.
func (s *Service) fetchProfile(ctx context.Context, code string) (*profile, error) {
	g := s.cfg.Google
	form := url.Values{
		"code":          {code},
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"redirect_uri":  {g.RedirectURL},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := s.doJSON(req, &tok); err != nil {
		return nil, fmt.Errorf("token: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("token: empty access_token")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, g.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	var p profile
	if err := s.doJSON(req, &p); err != nil {
		return nil, fmt.Errorf("userinfo: %w", err)
	}
	if p.Sub == "" {
		return nil, errors.New("userinfo: no sub")
	}
	return &p, nil
}

func (s *Service) doJSON(req *http.Request, out any) error {
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %.200s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

// signInGoogle finds or creates the user for a Google profile and returns
// their id, folding in the guest account the browser currently has.
func (s *Service) signInGoogle(r *http.Request, p *profile) (string, error) {
	ctx := r.Context()
	name := cleanName(p.Name, "Игрок")
	cur := s.UserFromRequest(r)
	if cur != nil && cur.Kind != store.KindGuest {
		cur = nil // already a Google user: signing in again is a plain switch
	}

	existing, err := s.cfg.Store.UserByGoogleSub(ctx, p.Sub)
	switch {
	case err == nil:
		if cur != nil && cur.ID != existing.ID {
			if err := s.cfg.Store.MergeUsers(ctx, cur.ID, existing.ID); err != nil {
				return "", err
			}
		}
		if err := s.cfg.Store.LinkGoogle(ctx, existing.ID, p.Sub, name, p.Email, p.Picture); err != nil {
			return "", err
		}
		return existing.ID, nil
	case !errors.Is(err, store.ErrNotFound):
		return "", err
	}

	if cur != nil { // upgrade the guest in place
		if err := s.cfg.Store.LinkGoogle(ctx, cur.ID, p.Sub, name, p.Email, p.Picture); err != nil {
			return "", err
		}
		return cur.ID, nil
	}
	u := &store.User{ID: randomID(), Kind: store.KindGoogle, GoogleSub: p.Sub, Name: name, Email: p.Email, AvatarURL: p.Picture}
	if err := s.cfg.Store.CreateUser(ctx, u); err != nil {
		return "", err
	}
	return u.ID, nil
}

// -------------------------------------------------------------- sessions

// startSession gives the browser a fresh session for the user. Any old
// session is dropped, so a token that was known before signing in is useless
// afterwards.
func (s *Service) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.cfg.Store.DeleteSession(r.Context(), hashToken(c.Value))
	}
	token := randomToken()
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.cfg.Store.CreateSession(r.Context(), hashToken(token), userID, expires); err != nil {
		return err
	}
	s.setCookie(w, r, sessionCookie, token, "/", int(s.cfg.SessionTTL.Seconds()))
	return nil
}

func (s *Service) setCookie(w http.ResponseWriter, r *http.Request, name, value, path string, maxAge int) {
	secure := s.cfg.CookieSecure || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: path, MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // the system RNG failing is not recoverable
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func guestName() string {
	n, err := rand.Int(rand.Reader, big.NewInt(9000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("Гость %d", 1000+n.Int64())
}

// cleanName trims a display name to the column size and drops control
// characters; fallback is used when nothing is left.
func cleanName(s, fallback string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r >= 0x20 && r != 0x7f && utf8.RuneLen(r) > 0 {
			b.WriteRune(r)
		}
	}
	out := []rune(strings.TrimSpace(b.String()))
	if len(out) > 100 {
		out = out[:100]
	}
	if len(out) == 0 {
		return fallback
	}
	return string(out)
}

// ------------------------------------------------------------ rate limit

// clientIP is the caller's address. nginx puts the real one in X-Real-IP;
// the server is not reachable except through it in the Docker setup.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limiter allows n events per window for each key.
type limiter struct {
	n      int
	window time.Duration
	mu     sync.Mutex
	hits   map[string][]time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, hits: map[string][]time.Time{}}
}

func (l *limiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.n {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	if len(l.hits) > 10000 { // forget idle keys so the map cannot grow forever
		for k, v := range l.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) >= l.window {
				delete(l.hits, k)
			}
		}
	}
	return true
}

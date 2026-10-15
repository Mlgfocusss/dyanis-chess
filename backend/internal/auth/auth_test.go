package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// memStore is an in-memory Store.
type memStore struct {
	mu       sync.Mutex
	users    map[string]*store.User
	sessions map[string]string // token hash -> user id
	merged   [][2]string
}

func newMemStore() *memStore {
	return &memStore{users: map[string]*store.User{}, sessions: map[string]string{}}
}

func (m *memStore) CreateUser(_ context.Context, u *store.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *u
	m.users[u.ID] = &c
	return nil
}

func (m *memStore) UserByGoogleSub(_ context.Context, sub string) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.GoogleSub == sub {
			c := *u
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *memStore) LinkGoogle(_ context.Context, id, sub, name, email, avatar string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.users[id]
	u.Kind, u.GoogleSub, u.Name, u.Email, u.AvatarURL = store.KindGoogle, sub, name, email, avatar
	return nil
}

func (m *memStore) MergeUsers(_ context.Context, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.merged = append(m.merged, [2]string{from, to})
	delete(m.users, from)
	return nil
}

func (m *memStore) SetSettings(_ context.Context, id string, settings []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[id].Settings = settings
	return nil
}

func (m *memStore) CreateSession(_ context.Context, hash, userID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[hash] = userID
	return nil
}

func (m *memStore) SessionUser(_ context.Context, hash string) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u := m.users[m.sessions[hash]]; u != nil {
		c := *u
		return &c, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) DeleteSession(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hash)
	return nil
}

// fakeGoogle plays Google's token and userinfo endpoints for one profile.
func fakeGoogle(t *testing.T, p profile) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "good-code" || r.Form.Get("client_secret") != "shh" ||
			r.Form.Get("grant_type") != "authorization_code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"access_token":"tok123","token_type":"Bearer"}`))
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(p)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T, db Store, google *httptest.Server) *Service {
	t.Helper()
	g := Google{ClientID: "cid", ClientSecret: "shh", RedirectURL: "http://site.test/api/auth/google/callback"}
	if google != nil {
		g.TokenURL, g.UserInfoURL = google.URL+"/token", google.URL+"/userinfo"
	}
	return New(Config{Store: db, Google: g})
}

// client is a tiny cookie jar around one handler.
type client struct {
	t       *testing.T
	h       http.Handler
	cookies map[string]*http.Cookie
}

func newClient(t *testing.T, h http.Handler) *client {
	return &client{t: t, h: h, cookies: map[string]*http.Cookie{}}
}

func (c *client) do(method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 || ck.Value == "" {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
	return rec
}

type meResp struct {
	User   *userJSON `json:"user"`
	Google bool      `json:"google"`
}

func (c *client) me() meResp {
	c.t.Helper()
	rec := c.do("GET", "/api/me", "")
	var r meResp
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil || rec.Code != 200 {
		c.t.Fatalf("/api/me: %d %s", rec.Code, rec.Body)
	}
	return r
}

func TestAnonymousMe(t *testing.T) {
	c := newClient(t, newService(t, newMemStore(), nil).Handler())
	if r := c.me(); r.User != nil || !r.Google {
		t.Errorf("anonymous: %+v", r)
	}
}

func TestGuestFlow(t *testing.T) {
	db := newMemStore()
	c := newClient(t, newService(t, db, nil).Handler())

	rec := c.do("POST", "/api/auth/guest", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("guest: %d %s", rec.Code, rec.Body)
	}
	ck := rec.Result().Cookies()[0]
	if ck.Name != sessionCookie || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.MaxAge < 24*3600 {
		t.Errorf("session cookie: %+v", ck)
	}
	u := c.me().User
	if u == nil || u.Kind != store.KindGuest || !strings.HasPrefix(u.Name, "Гость ") {
		t.Fatalf("me after guest: %+v", u)
	}
	// The database must not hold the raw token.
	for h := range db.sessions {
		if h == ck.Value || len(h) != 64 {
			t.Errorf("session key %q is not a SHA-256 hex", h)
		}
	}
	// Asking again keeps the same guest.
	rec = c.do("POST", "/api/auth/guest", "")
	var again meResp
	json.Unmarshal(rec.Body.Bytes(), &again)
	if rec.Code != 200 || again.User.ID != u.ID || len(db.users) != 1 {
		t.Errorf("second guest request: %d %+v users=%d", rec.Code, again.User, len(db.users))
	}

	rec = c.do("POST", "/api/auth/logout", "")
	if rec.Code != 200 || c.me().User != nil || len(db.sessions) != 0 {
		t.Errorf("after logout: %d sessions=%d", rec.Code, len(db.sessions))
	}
}

func TestGuestRateLimit(t *testing.T) {
	c := newClient(t, newService(t, newMemStore(), nil).Handler())
	got429 := false
	for i := 0; i < 40; i++ {
		c.cookies = map[string]*http.Cookie{} // a fresh browser each time
		if c.do("POST", "/api/auth/guest", "", "X-Real-IP", "203.0.113.9").Code == http.StatusTooManyRequests {
			got429 = true
			if i < 30 {
				t.Errorf("limited too early, at request %d", i)
			}
			break
		}
	}
	if !got429 {
		t.Error("no limit after 40 guests from one address")
	}
	// Another address is not affected.
	if rec := c.do("POST", "/api/auth/guest", "", "X-Real-IP", "203.0.113.10"); rec.Code != http.StatusCreated {
		t.Errorf("other address: %d", rec.Code)
	}
}

func TestSessionExpiryAndGarbageCookie(t *testing.T) {
	db := newMemStore()
	c := newClient(t, newService(t, db, nil).Handler())
	c.cookies[sessionCookie] = &http.Cookie{Name: sessionCookie, Value: "not-a-real-token"}
	if c.me().User != nil {
		t.Error("a made-up cookie signed someone in")
	}
}

// loginURL starts the Google flow and returns the state the server issued.
func startGoogle(t *testing.T, c *client) (state string) {
	t.Helper()
	rec := c.do("GET", "/api/auth/google/login", "")
	if rec.Code != http.StatusFound {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := loc.Query()
	if q.Get("client_id") != "cid" || q.Get("response_type") != "code" ||
		q.Get("redirect_uri") != "http://site.test/api/auth/google/callback" || !strings.Contains(q.Get("scope"), "profile") {
		t.Errorf("authorize URL: %s", loc)
	}
	state = q.Get("state")
	if len(state) < 20 || c.cookies[stateCookie] == nil || c.cookies[stateCookie].Value != state {
		t.Errorf("state %q does not match its cookie", state)
	}
	return state
}

func callback(c *client, state, code string) *httptest.ResponseRecorder {
	return c.do("GET", "/api/auth/google/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), "")
}

func TestGoogleNewUser(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-1", Name: "Анна Иванова", Email: "anna@example.com", Picture: "https://img/a.png"})
	c := newClient(t, newService(t, db, g).Handler())

	state := startGoogle(t, c)
	rec := callback(c, state, "good-code")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#account" {
		t.Fatalf("callback: %d -> %s", rec.Code, rec.Header().Get("Location"))
	}
	u := c.me().User
	if u == nil || u.Kind != store.KindGoogle || u.Name != "Анна Иванова" || u.Email != "anna@example.com" || u.AvatarURL != "https://img/a.png" {
		t.Fatalf("me: %+v", u)
	}
	if c.cookies[stateCookie] != nil {
		t.Error("the state cookie was not cleared")
	}
	// Signing in again (new browser) finds the same user.
	c2 := newClient(t, newService(t, db, g).Handler())
	callback(c2, startGoogle(t, c2), "good-code")
	if u2 := c2.me().User; u2 == nil || u2.ID != u.ID || len(db.users) != 1 {
		t.Errorf("second sign-in: %+v users=%d", u2, len(db.users))
	}
}

func TestGoogleUpgradesGuestInPlace(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-2", Name: "Борис"})
	c := newClient(t, newService(t, db, g).Handler())

	c.do("POST", "/api/auth/guest", "")
	guest := c.me().User
	oldToken := c.cookies[sessionCookie].Value
	c.do("PUT", "/api/me/settings", `{"boardTheme":"green"}`, "Content-Type", "application/json")

	callback(c, startGoogle(t, c), "good-code")
	u := c.me().User
	if u.ID != guest.ID || u.Kind != store.KindGoogle || u.Name != "Борис" {
		t.Errorf("guest was not upgraded in place: guest %+v now %+v", guest, u)
	}
	if string(u.Settings) != `{"boardTheme":"green"}` {
		t.Errorf("settings lost: %s", u.Settings)
	}
	if c.cookies[sessionCookie].Value == oldToken {
		t.Error("session token was not renewed at sign-in")
	}
	if _, ok := db.sessions[hashToken(oldToken)]; ok {
		t.Error("the guest's old session still works")
	}
}

func TestGoogleMergesGuestIntoKnownAccount(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-3", Name: "Вера"})
	first := newClient(t, newService(t, db, g).Handler())
	callback(first, startGoogle(t, first), "good-code")
	known := first.me().User

	c := newClient(t, newService(t, db, g).Handler())
	c.do("POST", "/api/auth/guest", "")
	guest := c.me().User
	callback(c, startGoogle(t, c), "good-code")

	if u := c.me().User; u.ID != known.ID {
		t.Errorf("signed in as %s, want the known account %s", u.ID, known.ID)
	}
	if len(db.merged) != 1 || db.merged[0] != [2]string{guest.ID, known.ID} {
		t.Errorf("merge calls: %v", db.merged)
	}
}

func TestGoogleRejects(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-4", Name: "X"})

	t.Run("wrong state", func(t *testing.T) {
		c := newClient(t, newService(t, db, g).Handler())
		startGoogle(t, c)
		rec := callback(c, "forged", "good-code")
		if loc := rec.Header().Get("Location"); loc != "/?auth_error=state#account" {
			t.Errorf("-> %s", loc)
		}
		if c.me().User != nil {
			t.Error("signed in with a forged state")
		}
	})
	t.Run("no state cookie", func(t *testing.T) {
		c := newClient(t, newService(t, db, g).Handler())
		if loc := callback(c, "anything", "good-code").Header().Get("Location"); loc != "/?auth_error=state#account" {
			t.Errorf("-> %s", loc)
		}
	})
	t.Run("state is single use", func(t *testing.T) {
		c := newClient(t, newService(t, db, g).Handler())
		state := startGoogle(t, c)
		callback(c, state, "good-code")
		c.do("POST", "/api/auth/logout", "")
		// The browser replays the same callback URL: the cookie is gone.
		if loc := callback(c, state, "good-code").Header().Get("Location"); loc != "/?auth_error=state#account" {
			t.Errorf("replayed callback -> %s", loc)
		}
	})
	t.Run("user pressed cancel", func(t *testing.T) {
		c := newClient(t, newService(t, db, g).Handler())
		state := startGoogle(t, c)
		rec := c.do("GET", "/api/auth/google/callback?state="+state+"&error=access_denied", "")
		if loc := rec.Header().Get("Location"); loc != "/?auth_error=denied#account" {
			t.Errorf("-> %s", loc)
		}
	})
	t.Run("google refuses the code", func(t *testing.T) {
		c := newClient(t, newService(t, db, g).Handler())
		rec := callback(c, startGoogle(t, c), "bad-code")
		if loc := rec.Header().Get("Location"); loc != "/?auth_error=google#account" {
			t.Errorf("-> %s", loc)
		}
		if c.me().User != nil {
			t.Error("signed in although Google refused")
		}
	})
}

func TestGoogleNotConfigured(t *testing.T) {
	svc := New(Config{Store: newMemStore()})
	c := newClient(t, svc.Handler())
	if rec := c.do("GET", "/api/auth/google/login", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("login without keys: %d", rec.Code)
	}
	if c.me().Google {
		t.Error("/api/me advertises Google without keys")
	}
}

func TestSettings(t *testing.T) {
	db := newMemStore()
	c := newClient(t, newService(t, db, nil).Handler())
	js := []string{"Content-Type", "application/json"}

	if rec := c.do("PUT", "/api/me/settings", `{"a":1}`, js...); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous PUT: %d", rec.Code)
	}
	c.do("POST", "/api/auth/guest", "")
	if rec := c.do("PUT", "/api/me/settings", `{"boardTheme":"brown","sound":false}`, js...); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	if got := string(c.me().User.Settings); got != `{"boardTheme":"brown","sound":false}` {
		t.Errorf("settings after PUT: %s", got)
	}
	cases := map[string]struct {
		body string
		code int
		hdr  []string
	}{
		"form content type": {`{"a":1}`, http.StatusUnsupportedMediaType, []string{"Content-Type", "text/plain"}},
		"not json":          {`nope`, http.StatusBadRequest, js},
		"array":             {`[1,2]`, http.StatusBadRequest, js},
		"null":              {`null`, http.StatusBadRequest, js},
		"too big":           {`{"a":"` + strings.Repeat("x", maxSettings) + `"}`, http.StatusRequestEntityTooLarge, js},
	}
	for name, tc := range cases {
		if rec := c.do("PUT", "/api/me/settings", tc.body, tc.hdr...); rec.Code != tc.code {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.code)
		}
	}
	if got := string(c.me().User.Settings); got != `{"boardTheme":"brown","sound":false}` {
		t.Errorf("a rejected PUT changed the settings: %s", got)
	}
}

func TestCleanName(t *testing.T) {
	long := strings.Repeat("я", 150)
	cases := []struct{ in, want string }{
		{"  Анна  ", "Анна"},
		{"", "Игрок"},
		{"   ", "Игрок"},
		{"a\x00b\nc", "abc"},
		{long, strings.Repeat("я", 100)},
	}
	for _, c := range cases {
		if got := cleanName(c.in, "Игрок"); got != c.want {
			t.Errorf("cleanName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGoogleReturnsToTheInvitation(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-9", Name: "Игорь"})
	c := newClient(t, newService(t, db, g).Handler())

	rec := c.do("GET", "/api/auth/google/login?next="+url.QueryEscape("#online/0123456789abcdef"), "")
	state := ""
	if loc, err := url.Parse(rec.Header().Get("Location")); err == nil {
		state = loc.Query().Get("state")
	}
	if rec = callback(c, state, "good-code"); rec.Header().Get("Location") != "/#online/0123456789abcdef" {
		t.Errorf("landed on %s", rec.Header().Get("Location"))
	}
	// The same for a refusal, so the person is not thrown out of the game page.
	c2 := newClient(t, newService(t, db, g).Handler())
	rec = c2.do("GET", "/api/auth/google/login?next="+url.QueryEscape("#online/abc"), "")
	loc, _ := url.Parse(rec.Header().Get("Location"))
	rec = c2.do("GET", "/api/auth/google/callback?state="+loc.Query().Get("state")+"&error=access_denied", "")
	if got := rec.Header().Get("Location"); got != "/?auth_error=denied#online/abc" {
		t.Errorf("refusal landed on %s", got)
	}

	// Anything that is not a hash route of this site is ignored (no open redirect).
	for _, bad := range []string{"https://evil.example/", "//evil.example", "/x", "#a b", "#", "javascript:alert(1)"} {
		c3 := newClient(t, newService(t, db, g).Handler())
		rec := c3.do("GET", "/api/auth/google/login?next="+url.QueryEscape(bad), "")
		loc, _ := url.Parse(rec.Header().Get("Location"))
		rec = callback(c3, loc.Query().Get("state"), "good-code")
		if got := rec.Header().Get("Location"); got != "/#account" {
			t.Errorf("next=%q redirected to %s", bad, got)
		}
	}
}

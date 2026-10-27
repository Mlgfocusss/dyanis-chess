package sameperson

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// memStore keeps the marks in memory with the same rules as the SQL of store/devices.go.
type memStore struct {
	mu    sync.Mutex
	marks map[string]map[store.DeviceMark]time.Time // user -> mark -> last seen
	calls int
}

func (m *memStore) TouchDevices(_ context.Context, u string, marks []store.DeviceMark, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.marks == nil {
		m.marks = map[string]map[store.DeviceMark]time.Time{}
	}
	if m.marks[u] == nil {
		m.marks[u] = map[store.DeviceMark]time.Time{}
	}
	for _, k := range marks {
		m.marks[u][k] = now
	}
	return nil
}

func (m *memStore) share(a, b string, dev, ip time.Time) bool {
	for k, at := range m.marks[a] {
		since := dev
		if k.Kind == store.DeviceIP {
			since = ip
		}
		if bt, ok := m.marks[b][k]; ok && at.After(since) && bt.After(since) {
			return true
		}
	}
	return false
}

func (m *memStore) SharingUsers(_ context.Context, u string, dev, ip time.Time) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for o := range m.marks {
		if o != u && m.share(u, o, dev, ip) {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m *memStore) ShareDevice(_ context.Context, a, b string, dev, ip time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.share(a, b, dev, ip), nil
}

type env struct {
	svc *Service
	st  *memStore
	now time.Time
}

// cookieOf is the value of the cookie name in the request, "" when there is none.
func cookieOf(r *http.Request, name string) string {
	if c, err := r.Cookie(name); err == nil {
		return c.Value
	}
	return ""
}

func newEnv() *env {
	e := &env{st: &memStore{}, now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	e.svc = New(Config{
		Store:   e.st,
		Session: func(r *http.Request) string { return cookieOf(r, "s") },
		User:    func(r *http.Request) string { return cookieOf(r, "s") },
		Now:     func() time.Time { return e.now },
	})
	return e
}

// visit makes a request to the API as session user from ip, with the device cookie dev ("" for none),
// and returns the device cookie the browser has afterwards.
func (e *env) visit(user, ip, dev string) string {
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("X-Real-IP", ip)
	r.AddCookie(&http.Cookie{Name: "s", Value: user})
	if dev != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: dev})
	}
	w := httptest.NewRecorder()
	e.svc.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			return c.Value
		}
	}
	return dev
}

func TestEveryVisitorGetsADeviceCookieOnce(t *testing.T) {
	e := newEnv()
	dev := e.visit("anna", "1.1.1.1", "")
	if len(dev) != 32 {
		t.Fatalf("cookie %q", dev)
	}
	if again := e.visit("anna", "1.1.1.1", dev); again != dev {
		t.Errorf("a visitor with a cookie must keep it, got %q", again)
	}
}

func TestOneBrowserTwoAccounts(t *testing.T) {
	e := newEnv()
	dev := e.visit("anna", "1.1.1.1", "")
	e.visit("anna2", "2.2.2.2", dev) // another address, same browser: signed out and in as a new guest
	e.visit("boris", "3.3.3.3", "")
	ctx := context.Background()
	if !e.svc.Same(ctx, "anna", "anna2") || !e.svc.Same(ctx, "anna2", "anna") {
		t.Error("the same browser is one person")
	}
	if e.svc.Same(ctx, "anna", "boris") {
		t.Error("strangers")
	}
	others, _ := e.svc.Others(ctx, "anna")
	if len(others) != 1 || !others["anna2"] {
		t.Errorf("others: %v", others)
	}
}

func TestOneNetworkOnlyWhileFresh(t *testing.T) {
	e := newEnv()
	e.visit("anna", "10.0.0.5", "")
	e.visit("boris", "10.0.0.5", "")
	ctx := context.Background()
	if !e.svc.Same(ctx, "anna", "boris") {
		t.Error("same address, same day")
	}
	e.now = e.now.Add(IPWindow + time.Hour) // the address is stale now
	if e.svc.Same(ctx, "anna", "boris") {
		t.Error("an old address proves nothing")
	}
}

func TestSameDoesNotAcceptTheSameIdOrNobody(t *testing.T) {
	e := newEnv()
	if e.svc.Same(context.Background(), "anna", "anna") || e.svc.Same(context.Background(), "", "anna") {
		t.Error("same id / nobody")
	}
}

func TestTheDatabaseIsNotAskedForEveryRequest(t *testing.T) {
	e := newEnv()
	dev := e.visit("anna", "1.1.1.1", "")
	for i := 0; i < 20; i++ {
		e.visit("anna", "1.1.1.1", dev)
	}
	if e.st.calls != 1 {
		t.Errorf("writes: %d", e.st.calls)
	}
	e.now = e.now.Add(2 * time.Hour)
	e.visit("anna", "1.1.1.1", dev)
	if e.st.calls != 2 {
		t.Errorf("an hour later: %d", e.st.calls)
	}
	e.visit("anna", "9.9.9.9", dev) // a new address is written down at once
	if e.st.calls != 3 {
		t.Errorf("new address: %d", e.st.calls)
	}
}

func TestIPv6IsCutToItsNetwork(t *testing.T) {
	mk := func(a string) string {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-Real-IP", a)
		return ipOf(r)
	}
	if mk("2001:db8:1:2:aaaa::1") != mk("2001:db8:1:2:bbbb::9") {
		t.Error("one /64 is one home")
	}
	if mk("2001:db8:1:3::1") == mk("2001:db8:1:2::1") {
		t.Error("different networks")
	}
	if mk("not an ip") != "" {
		t.Error("garbage must give nothing")
	}
}

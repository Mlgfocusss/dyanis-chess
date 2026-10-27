// Package sameperson guards the ratings against one person playing against themselves with two
// accounts. Every visitor gets a device cookie; the site remembers (as hashes) which cookies and
// which network addresses each signed-in account was seen with. Two accounts that share a cookie,
// or an address seen lately, count as one person: matchmaking does not pair them for a rated game or
// a duel, and a rated game or duel between them does not move either rating. Casual games are not
// affected. Friends who share a home network can play casually; this is the price of the check.
package sameperson

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"dyanis-chess/backend/internal/store"
)

// How long a trace counts. A device cookie lives for years and identifies a browser; an address is
// shared by neighbours, offices and mobile networks, so it only counts while it is fresh.
const (
	DeviceWindow = 90 * 24 * time.Hour
	IPWindow     = 3 * 24 * time.Hour

	cookieName = "dyanis_dev"
	cookieAge  = 2 * 365 * 24 * 3600
	touchEvery = time.Hour // the same session from the same place is written down this often
)

// Store is the part of *store.Store the service uses.
type Store interface {
	TouchDevices(ctx context.Context, userID string, marks []store.DeviceMark, now time.Time) error
	SharingUsers(ctx context.Context, userID string, deviceSince, ipSince time.Time) ([]string, error)
	ShareDevice(ctx context.Context, a, b string, deviceSince, ipSince time.Time) (bool, error)
}

// Config of a Service.
type Config struct {
	Store Store
	// Session names the session of a request ("" for none); it only has to tell sessions apart.
	Session func(r *http.Request) string
	// User is the id of the signed-in person of a request ("" for nobody).
	User         func(r *http.Request) string
	CookieSecure bool
	Now          func() time.Time
}

// Service remembers and compares.
type Service struct {
	cfg  Config
	mu   sync.Mutex
	seen map[string]time.Time // session+device+address -> when it was written down
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, seen: map[string]time.Time{}}
}

// Middleware gives every visitor a device cookie and writes down what the signed-in ones were seen with.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dev := s.ensureDevice(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.touch(r, dev)
		}
		next.ServeHTTP(w, r)
	})
}

// ensureDevice returns the device cookie of the request, issuing one when there is none.
func (s *Service) ensureDevice(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil && len(c.Value) == 32 {
		return c.Value
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	v := hex.EncodeToString(b[:])
	secure := s.cfg.CookieSecure || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: v, Path: "/", MaxAge: cookieAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	r.AddCookie(&http.Cookie{Name: cookieName, Value: v}) // later handlers of this request see it too
	return v
}

func (s *Service) touch(r *http.Request, dev string) {
	if dev == "" || s.cfg.Session == nil || s.cfg.User == nil {
		return
	}
	sess := s.cfg.Session(r)
	if sess == "" {
		return
	}
	ip := ipOf(r)
	key := sess + "|" + dev + "|" + ip
	now := s.cfg.Now()
	s.mu.Lock()
	if at, ok := s.seen[key]; ok && now.Sub(at) < touchEvery {
		s.mu.Unlock()
		return
	}
	if len(s.seen) > 20000 { // forget the old ones
		for k, at := range s.seen {
			if now.Sub(at) >= touchEvery {
				delete(s.seen, k)
			}
		}
	}
	s.seen[key] = now
	s.mu.Unlock()
	uid := s.cfg.User(r)
	if uid == "" {
		return
	}
	marks := []store.DeviceMark{{Kind: store.DeviceCookie, Hash: hash("dev", dev)}}
	if ip != "" {
		marks = append(marks, store.DeviceMark{Kind: store.DeviceIP, Hash: hash("ip", ip)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.cfg.Store.TouchDevices(ctx, uid, marks, now); err != nil {
		log.Printf("sameperson: touch %s: %v", uid, err)
		s.mu.Lock()
		delete(s.seen, key) // try again with the next request
		s.mu.Unlock()
	}
}

// Same says whether the two accounts look like one person. A failure of the database says no: better
// to count a game than to lose it.
func (s *Service) Same(ctx context.Context, a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	now := s.cfg.Now()
	ok, err := s.cfg.Store.ShareDevice(ctx, a, b, now.Add(-DeviceWindow), now.Add(-IPWindow))
	if err != nil {
		log.Printf("sameperson: %s and %s: %v", a, b, err)
		return false
	}
	return ok
}

// Others returns the accounts that look like the same person as the user.
func (s *Service) Others(ctx context.Context, userID string) (map[string]bool, error) {
	now := s.cfg.Now()
	ids, err := s.cfg.Store.SharingUsers(ctx, userID, now.Add(-DeviceWindow), now.Add(-IPWindow))
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// hash is a short digest of a value; the raw cookie and address are not stored.
func hash(kind, v string) string {
	h := sha256.Sum256([]byte("dyanis-chess:" + kind + ":" + v))
	return hex.EncodeToString(h[:16])
}

// ipOf is the address of the caller; nginx puts the real one in X-Real-IP. IPv6 addresses are cut to
// their /64 network, which is what one home gets.
func ipOf(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip = host
	}
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil {
		return ""
	}
	if v4 := p.To4(); v4 != nil {
		return v4.String()
	}
	return p.Mask(net.CIDRMask(64, 128)).String()
}

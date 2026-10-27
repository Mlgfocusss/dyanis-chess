package explorer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

// a Lichess that counts the calls and shows what it was asked
type fakeLichess struct {
	calls  atomic.Int32
	mu     sync.Mutex
	last   url.Values
	auth   string
	status int
	retry  string
	body   string
}

func (f *fakeLichess) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.mu.Lock()
	f.last, f.auth = r.URL.Query(), r.Header.Get("Authorization")
	status, retry, body := f.status, f.retry, f.body
	f.mu.Unlock()
	if retry != "" {
		w.Header().Set("Retry-After", retry)
	}
	if status != 0 {
		w.WriteHeader(status)
	}
	w.Write([]byte(body))
}

const startAnswer = `{"white":600,"draws":100,"black":300,"moves":[
 {"uci":"e2e4","san":"e4","averageRating":1700,"white":300,"draws":50,"black":150,"opening":{"eco":"B00","name":"King's Pawn Game"}},
 {"uci":"d2d4","san":"d4","averageRating":1710,"white":200,"draws":40,"black":100}],
 "topGames":[],"recentGames":[]}`

func setup(token string) (*fakeLichess, *Service, *time.Time) {
	f := &fakeLichess{body: startAnswer}
	srv := httptest.NewServer(f)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	s := New(Config{Token: token, BaseURL: srv.URL, UserID: func(*http.Request) string { return "anna" }, Now: func() time.Time { return now }})
	return f, s, &now
}

func get(s *Service, query string) (int, Response, http.Header) {
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/explorer?"+query, nil))
	var out Response
	if rec.Code == 200 {
		json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out, rec.Header()
}

func q(fen string, extra ...string) string {
	v := url.Values{"fen": {fen}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return v.Encode()
}

func TestAnswerIsNormalized(t *testing.T) {
	f, s, _ := setup("tok123")
	code, out, _ := get(s, q(startFEN))
	if code != 200 || !out.Available || out.Total != 1000 || out.White != 600 || len(out.Moves) != 2 {
		t.Fatalf("%d %+v", code, out)
	}
	e4 := out.Moves[0]
	if e4.UCI != "e2e4" || e4.SAN != "e4" || e4.Games != 500 || e4.White != 300 || e4.AverageRating != 1700 || e4.Opening == nil || e4.Opening.Name != "King's Pawn Game" {
		t.Errorf("e4: %+v", e4)
	}
	if f.auth != "Bearer tok123" {
		t.Errorf("authorization %q", f.auth)
	}
	if f.last.Get("fen") != startFEN || f.last.Get("variant") != "standard" || f.last.Get("topGames") != "0" || f.last.Get("moves") != "12" {
		t.Errorf("request %v", f.last)
	}
	// the defaults: middle ratings, not bullet
	if f.last.Get("ratings") != "1600,1800" || f.last.Get("speeds") != "blitz,rapid,classical" {
		t.Errorf("defaults %v", f.last)
	}
}

func TestParametersAreCleaned(t *testing.T) {
	f, s, _ := setup("tok")
	get(s, q(startFEN, "ratings", "1400, 1600,evil=1,999,1400", "speeds", "rapid,bullet;drop"))
	if f.last.Get("ratings") != "1400,1600" || f.last.Get("speeds") != "rapid" {
		t.Errorf("sent %v", f.last)
	}
	if code, _, _ := get(s, q(startFEN, "ratings", "123")); code != 400 {
		t.Errorf("only unknown ratings: %d", code)
	}
	if code, _, _ := get(s, q("nonsense")); code != 400 {
		t.Errorf("bad fen: %d", code)
	}
}

func TestAnswersAreRemembered(t *testing.T) {
	f, s, now := setup("tok")
	for i := 0; i < 3; i++ {
		get(s, q(startFEN))
	}
	if f.calls.Load() != 1 {
		t.Errorf("%d calls for the same question", f.calls.Load())
	}
	// another rating range is another question
	get(s, q(startFEN, "ratings", "2200"))
	if f.calls.Load() != 2 {
		t.Errorf("%d calls", f.calls.Load())
	}
	// a day later the answer is stale
	*now = now.Add(25 * time.Hour)
	get(s, q(startFEN))
	if f.calls.Load() != 3 {
		t.Errorf("%d calls after a day", f.calls.Load())
	}
}

func TestWithoutATokenOrAnAccount(t *testing.T) {
	f, s, _ := setup("")
	code, out, _ := get(s, q(startFEN))
	if code != 200 || out.Available || len(out.Moves) != 0 || f.calls.Load() != 0 {
		t.Errorf("no token: %d %+v, %d calls", code, out, f.calls.Load())
	}
	s2 := New(Config{Token: "tok", BaseURL: "http://127.0.0.1:1"}) // nobody signed in
	if code, _, _ := get(s2, q(startFEN)); code != 401 {
		t.Errorf("nobody signed in: %d", code)
	}
}

func TestLichessAsksToWait(t *testing.T) {
	f, s, now := setup("tok")
	f.status, f.retry = http.StatusTooManyRequests, "30"
	code, _, h := get(s, q(startFEN))
	if code != 429 || h.Get("Retry-After") != "31" {
		t.Fatalf("%d, Retry-After %q", code, h.Get("Retry-After"))
	}
	// while it asks to be left alone, nothing is sent (even for another position)
	before := f.calls.Load()
	if code, _, _ := get(s, q("rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1")); code != 429 {
		t.Errorf("during the pause: %d", code)
	}
	if f.calls.Load() != before {
		t.Error("a request went to Lichess during the pause")
	}
	// after the pause it works again
	*now = now.Add(31 * time.Second)
	f.mu.Lock()
	f.status, f.retry = 0, ""
	f.mu.Unlock()
	if code, out, _ := get(s, q(startFEN)); code != 200 || !out.Available {
		t.Errorf("after the pause: %d %+v", code, out)
	}
}

func TestUpstreamErrors(t *testing.T) {
	f, s, _ := setup("tok")
	f.status = http.StatusUnauthorized
	if code, _, _ := get(s, q(startFEN)); code != 502 {
		t.Errorf("a refused token: %d", code)
	}
	f.mu.Lock()
	f.status, f.body = 0, "not json"
	f.mu.Unlock()
	if code, _, _ := get(s, q(startFEN)); code != 502 {
		t.Errorf("a broken answer: %d", code)
	}
	if len(s.cache) != 0 {
		t.Error("an error was remembered")
	}
}

func TestCastlingIsWrittenTheEnginesWay(t *testing.T) {
	cases := [][3]string{
		{"e1h1", "O-O", "e1g1"}, {"e1a1", "O-O-O", "e1c1"},
		{"e8h8", "O-O", "e8g8"}, {"e8a8", "O-O-O", "e8c8"},
		{"e1g1", "O-O", "e1g1"}, // already the king's move
		{"e2e4", "e4", "e2e4"}, {"g1f3", "Nf3", "g1f3"},
	}
	for _, c := range cases {
		if got := fixCastling(c[0], c[1]); got != c[2] {
			t.Errorf("%s %s: %s, want %s", c[0], c[1], got, c[2])
		}
	}
	if out := normalize(upstream{}); !out.Available || out.Moves == nil {
		t.Error("an empty answer is still an answer with a list")
	}
}

func TestCacheIsBounded(t *testing.T) {
	_, s, _ := setup("tok")
	s.cfg.MaxCache = 3
	for i := 0; i < 10; i++ {
		s.remember(string(rune('a'+i)), Response{Available: true})
	}
	if len(s.cache) > 3 {
		t.Errorf("%d answers kept", len(s.cache))
	}
}

// ---------------------------------------------- Lookup, for code on the server

func TestLookupUsesTheCacheAndSharesItWithTheEndpoint(t *testing.T) {
	f, s, _ := setup("tok123")
	ratings, speeds, ok := Params("", "")
	if !ok || ratings != defaultRatings || speeds != defaultSpeeds {
		t.Fatalf("params %q %q %v", ratings, speeds, ok)
	}
	if _, ok := s.Cached(startFEN, ratings, speeds); ok {
		t.Fatal("a cached answer before any request")
	}
	resp, cached, err := s.Lookup(context.Background(), startFEN, ratings, speeds)
	if err != nil || cached || !resp.Available || resp.Total != 1000 || len(resp.Moves) != 2 {
		t.Fatalf("first lookup: %+v cached=%v err=%v", resp, cached, err)
	}
	if _, cached, _ := s.Lookup(context.Background(), startFEN, ratings, speeds); !cached {
		t.Error("the second lookup was not cached")
	}
	if _, ok := s.Cached(startFEN, ratings, speeds); !ok {
		t.Error("Cached does not see the answer")
	}
	// the endpoint finds the same entry
	if code, _, _ := get(s, q(startFEN)); code != 200 || f.calls.Load() != 1 {
		t.Errorf("the endpoint asked Lichess again: %d requests", f.calls.Load())
	}
	if _, _, err := s.Lookup(context.Background(), "not a fen", ratings, speeds); err == nil {
		t.Error("a bad position was accepted")
	}
	if _, _, ok := Params("9999", ""); ok {
		t.Error("an unknown rating group was accepted")
	}
}

func TestLookupWaitsWhenLichessAsksAndWithoutToken(t *testing.T) {
	f, s, now := setup("tok123")
	f.status, f.retry = 429, "30"
	_, _, err := s.Lookup(context.Background(), startFEN, defaultRatings, defaultSpeeds)
	var wait *WaitError
	if !errors.As(err, &wait) || wait.Wait != 30*time.Second {
		t.Fatalf("lookup under a limit: %v", err)
	}
	// until the time is up, nobody is asked
	if _, _, err := s.Lookup(context.Background(), startFEN, defaultRatings, defaultSpeeds); !errors.As(err, &wait) || f.calls.Load() != 1 {
		t.Errorf("asked during the pause: %v after %d requests", err, f.calls.Load())
	}
	*now = now.Add(time.Minute)
	f.mu.Lock()
	f.status, f.retry = 0, ""
	f.mu.Unlock()
	if _, cached, err := s.Lookup(context.Background(), startFEN, defaultRatings, defaultSpeeds); err != nil || cached {
		t.Errorf("after the pause: cached=%v err=%v", cached, err)
	}

	_, off, _ := setup("")
	if off.Enabled() {
		t.Error("enabled without a token")
	}
	if resp, _, err := off.Lookup(context.Background(), startFEN, defaultRatings, defaultSpeeds); err != nil || resp.Available {
		t.Errorf("without a token: %+v %v", resp, err)
	}
}

package ratings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

type memStore struct{ rows map[string]*store.RatingRow }

func key(u, c string) string { return u + "/" + c }

func (m *memStore) Rating(_ context.Context, u, c string) (*store.RatingRow, error) {
	if r, ok := m.rows[key(u, c)]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) Ratings(_ context.Context, u string) ([]*store.RatingRow, error) {
	var out []*store.RatingRow
	for _, r := range m.rows {
		if r.UserID == u {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) SaveRating(_ context.Context, x *store.RatingRow) error {
	cp := *x
	m.rows[key(x.UserID, x.Category)] = &cp
	return nil
}

func TestCategoryFor(t *testing.T) {
	for _, c := range []struct {
		base, inc int64
		want      string
	}{
		{60_000, 0, Bullet}, {120_000, 1000, Bullet}, // 2 min + 40 s = 160 s
		{180_000, 0, Blitz}, {180_000, 2000, Blitz}, {300_000, 0, Blitz}, {420_000, 0, Blitz},
		{600_000, 0, Rapid}, {900_000, 10_000, Rapid}, {3 * 3600_000, 0, Rapid},
		{0, 0, ""},
	} {
		if got := CategoryFor(c.base, c.inc); got != c.want {
			t.Errorf("%d+%d: %q, want %q", c.base/1000, c.inc/1000, got, c.want)
		}
	}
}

func TestApplyIsZeroSumForEqualNewcomersAndCountsGames(t *testing.T) {
	ctx := context.Background()
	m := &memStore{rows: map[string]*store.RatingRow{}}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	s := New(m, func() time.Time { return now })

	a, b, err := s.Apply(ctx, Blitz, "anna", "boris", 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.Before != 1200 || b.Before != 1200 || a.After <= 1200 || b.After >= 1200 || !a.Provisional {
		t.Fatalf("anna %+v boris %+v", a, b)
	}
	if a.After-a.Before != b.Before-b.After {
		t.Errorf("not symmetric: %+v %+v", a, b)
	}
	e, _ := s.Get(ctx, "anna", Blitz)
	if e.Games != 1 || e.Wins != 1 || e.Rating != a.After {
		t.Errorf("anna: %+v", e)
	}
	if other, _ := s.Get(ctx, "anna", Rapid); other.Games != 0 || other.Rating != 1200 {
		t.Errorf("another category moved: %+v", other)
	}
	// a draw and a loss are counted
	s.Apply(ctx, Blitz, "anna", "boris", 0.5)
	s.Apply(ctx, Blitz, "anna", "boris", 0)
	e, _ = s.Get(ctx, "anna", Blitz)
	if e.Games != 3 || e.Wins != 1 || e.Draws != 1 || e.Losses != 1 {
		t.Errorf("record: %+v", e)
	}
	// a person cannot play themselves
	if _, _, err := s.Apply(ctx, Blitz, "anna", "anna", 1); err == nil {
		t.Error("self game accepted")
	}
}

func TestAwayFromTheBoardRaisesUncertainty(t *testing.T) {
	ctx := context.Background()
	m := &memStore{rows: map[string]*store.RatingRow{}}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	s := New(m, func() time.Time { return now })
	for i := 0; i < 30; i++ { // enough games to settle
		s.Apply(ctx, Rapid, "anna", "boris", float64(i%2))
	}
	before, _ := s.Get(ctx, "anna", Rapid)
	now = now.Add(200 * 24 * time.Hour)
	after, _ := s.Get(ctx, "anna", Rapid)
	if after.RD <= before.RD || after.Rating != before.Rating {
		t.Errorf("200 days away: %+v then %+v", before, after)
	}
}

func TestHandler(t *testing.T) {
	m := &memStore{rows: map[string]*store.RatingRow{}}
	s := New(m, nil)
	s.Apply(context.Background(), Duel, "anna", "boris", 1)
	get := func(user string) *httptest.ResponseRecorder {
		h := s.Handler(func(*http.Request) string { return user })
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/ratings", nil))
		return rec
	}
	rec := get("anna")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"category":"duel"`) || !strings.Contains(rec.Body.String(), `"category":"bullet"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := get(""); rec.Code != 401 {
		t.Errorf("nobody: %d", rec.Code)
	}
}

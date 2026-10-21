package profiles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// mem is a store in memory: ratings, accounts and bot records.
type mem struct {
	rows  map[string]*store.RatingRow // user/category
	users map[string]*store.PublicUser
	bots  map[string][]store.BotRecord
	games map[string]int
}

func newMem() *mem {
	return &mem{rows: map[string]*store.RatingRow{}, users: map[string]*store.PublicUser{}, bots: map[string][]store.BotRecord{}, games: map[string]int{}}
}

func (m *mem) Rating(_ context.Context, u, c string) (*store.RatingRow, error) {
	if r, ok := m.rows[u+"/"+c]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, store.ErrNotFound
}

func (m *mem) Ratings(_ context.Context, u string) ([]*store.RatingRow, error) { return nil, nil }

func (m *mem) SaveRating(_ context.Context, x *store.RatingRow) error {
	cp := *x
	m.rows[x.UserID+"/"+x.Category] = &cp
	return nil
}

func (m *mem) LeaderCandidates(_ context.Context, category string, minGames int, maxRD float64, limit int, country string) ([]store.LeaderRow, error) {
	var out []store.LeaderRow
	for _, r := range m.rows {
		if country != "" && m.users[r.UserID].Country != country {
			continue
		}
		if r.Category == category && r.Games >= minGames && r.RD <= maxRD {
			out = append(out, store.LeaderRow{UserID: r.UserID, Rating: r.Rating, RD: r.RD, Volatility: r.Volatility, Games: r.Games, UpdatedAt: r.UpdatedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rating != out[j].Rating {
			return out[i].Rating > out[j].Rating
		}
		return out[i].UserID < out[j].UserID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *mem) PublicUser(_ context.Context, id string) (*store.PublicUser, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, store.ErrNotFound
}

func (m *mem) PublicUsers(_ context.Context, ids []string) (map[string]*store.PublicUser, error) {
	out := map[string]*store.PublicUser{}
	for _, id := range ids {
		if u, ok := m.users[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

func (m *mem) BotProgress(_ context.Context, u string) ([]store.BotRecord, error) {
	return m.bots[u], nil
}
func (m *mem) CountEndedGames(_ context.Context, u string) (int, error) { return m.games[u], nil }

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// add puts a person with a rating in a category: the stored deviation rd, and the last game at t0.
func (m *mem) add(id, name, category string, rating, rd float64, games int) {
	if _, ok := m.users[id]; !ok {
		m.users[id] = &store.PublicUser{ID: id, Kind: store.KindGuest, Name: name, CreatedAt: t0}
	}
	m.rows[id+"/"+category] = &store.RatingRow{UserID: id, Category: category, Rating: rating, RD: rd, Volatility: 0.06, Games: games, UpdatedAt: t0}
}

func service(m *mem, now time.Time) *Service {
	s := New(m, ratings.New(m, func() time.Time { return now }), func() time.Time { return now })
	s.ttl = 0
	return s
}

func TestLeaderboardOrderAndFilters(t *testing.T) {
	m := newMem()
	m.add("a", "Анна", ratings.Blitz, 1500, 60, 30)
	m.add("b", "Борис", ratings.Blitz, 1700, 80, 12)
	m.add("c", "Вера", ratings.Blitz, 1900, 60, 4)   // too few games
	m.add("d", "Глеб", ratings.Blitz, 2000, 200, 40) // rough rating
	m.add("e", "Дина", ratings.Blitz, 1500, 70, 9)   // ties with Анна: by id
	m.add("f", "Егор", ratings.Rapid, 2100, 50, 50)  // another category
	s := service(m, t0)

	rows, err := s.Leaderboard(context.Background(), ratings.Blitz, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b", "a", "e"}
	if len(rows) != len(want) {
		t.Fatalf("%+v", rows)
	}
	for i, id := range want {
		if rows[i].ID != id || rows[i].Place != i+1 {
			t.Errorf("row %d: %+v, want %s", i, rows[i], id)
		}
	}
	if rows[0].Rating != 1700 || rows[0].Games != 12 || rows[0].Name != "Борис" {
		t.Errorf("first: %+v", rows[0])
	}
	if rows, _ = s.Leaderboard(context.Background(), ratings.Blitz, "", 2); len(rows) != 2 {
		t.Errorf("limit 2 gave %d", len(rows))
	}
	if rows, _ = s.Leaderboard(context.Background(), ratings.Bullet, "", 50); len(rows) != 0 {
		t.Errorf("empty category: %+v", rows)
	}
}

// A person away for long has a grown deviation and leaves the board.
func TestIdleDaysTakeAPlayerOffTheBoard(t *testing.T) {
	m := newMem()
	m.add("a", "Анна", ratings.Rapid, 1500, 105, 30)
	m.add("b", "Борис", ratings.Rapid, 1400, 40, 30)
	rows, _ := service(m, t0).Leaderboard(context.Background(), ratings.Rapid, "", 50)
	if len(rows) != 2 {
		t.Fatalf("today: %+v", rows)
	}
	rows, _ = service(m, t0.Add(60*24*time.Hour)).Leaderboard(context.Background(), ratings.Rapid, "", 50)
	if len(rows) != 1 || rows[0].ID != "b" {
		t.Errorf("60 days later: %+v", rows)
	}
}

func get(h http.Handler, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	return rec
}

func TestLeaderboardHandler(t *testing.T) {
	m := newMem()
	for i, name := range []string{"Ира", "Петя", "Саша"} {
		m.add(string(rune('a'+i)), name, ratings.Duel, float64(1300+100*i), 50, 10)
	}
	h := service(m, t0).Handler()

	rec := get(h, "/api/leaderboard?category=duel")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var body struct {
		Category string `json:"category"`
		Rows     []Row  `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Category != "duel" || len(body.Rows) != 3 || body.Rows[0].Name != "Саша" || body.Rows[0].Place != 1 {
		t.Errorf("%+v", body)
	}
	if strings.Contains(rec.Body.String(), "email") {
		t.Errorf("private field in %s", rec.Body)
	}
	if rec := get(h, "/api/leaderboard?category=duel&limit=1"); strings.Count(rec.Body.String(), `"place"`) != 1 {
		t.Errorf("limit: %s", rec.Body)
	}
	if rec := get(h, "/api/leaderboard?category=duel&limit=100000"); rec.Code != 200 {
		t.Errorf("a huge limit is cut, not refused: %d", rec.Code)
	}
	for _, bad := range []string{"category=chess", "category=duel&limit=0", "category=duel&limit=x"} {
		if rec := get(h, "/api/leaderboard?"+bad); rec.Code != 400 {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	if rec := get(h, "/api/leaderboard"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"category":"blitz"`) {
		t.Errorf("default category: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/leaderboard"); !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Errorf("empty board must be a list: %s", rec.Body)
	}
}

func TestProfile(t *testing.T) {
	m := newMem()
	m.add("a", "Анна", ratings.Blitz, 1800, 50, 20)
	m.add("b", "Борис", ratings.Blitz, 1600, 50, 20)
	m.add("b", "Борис", ratings.Rapid, 1234, 50, 3) // too few games for a place
	m.games["b"] = 41
	m.bots["b"] = []store.BotRecord{{BotID: "x", Wins: 2, Points: 30}, {BotID: "y", Losses: 1}}
	h := service(m, t0).Handler()

	rec := get(h, "/api/users/b")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var p Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Борис" || p.Kind != "guest" || p.Games != 41 || p.Bots.Points != 30 || p.Bots.Beaten != 1 || p.Bots.Total < 2 {
		t.Errorf("%+v", p)
	}
	if len(p.Ratings) != 4 {
		t.Fatalf("every category is listed: %+v", p.Ratings)
	}
	by := map[string]RatingJSON{}
	for _, r := range p.Ratings {
		by[r.Category] = r
	}
	if by["blitz"].Place != 2 || by["blitz"].Rating != 1600 || by["rapid"].Place != 0 || by["rapid"].Games != 3 || by["bullet"].Games != 0 {
		t.Errorf("%+v", by)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "email") || strings.Contains(rec.Body.String(), "settings") {
		t.Errorf("private field in %s", rec.Body)
	}
	if rec := get(h, "/api/users/nobody"); rec.Code != 404 {
		t.Errorf("unknown: %d", rec.Code)
	}
	if rec := get(h, "/api/users/"+strings.Repeat("x", 100)); rec.Code != 404 {
		t.Errorf("long id: %d", rec.Code)
	}
}

func TestLeaderboardByCountry(t *testing.T) {
	m := newMem()
	m.add("a", "Анна", ratings.Blitz, 1500, 60, 30)
	m.add("b", "Борис", ratings.Blitz, 1700, 60, 30)
	m.add("c", "Вера", ratings.Blitz, 1900, 60, 30)
	m.add("d", "Глеб", ratings.Blitz, 1600, 60, 30)
	m.users["a"].Country, m.users["b"].Country, m.users["c"].Country = "BY", "BY", "PL"
	h := service(m, t0).Handler()

	rec := get(h, "/api/leaderboard?category=blitz&country=BY")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var body struct {
		Country string `json:"country"`
		Rows    []Row  `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Country != "BY" || len(body.Rows) != 2 || body.Rows[0].ID != "b" || body.Rows[1].ID != "a" || body.Rows[0].Place != 1 || body.Rows[1].Place != 2 {
		t.Errorf("Belarus: %+v", body)
	}
	for _, r := range body.Rows {
		if r.Country != "BY" {
			t.Errorf("row of another country: %+v", r)
		}
	}
	// without the filter everybody is there and each row carries the code of the country, if any
	rec = get(h, "/api/leaderboard?category=blitz")
	var all struct {
		Rows []Row `json:"rows"`
	}
	json.Unmarshal(rec.Body.Bytes(), &all)
	if len(all.Rows) != 4 || all.Rows[0].ID != "c" || all.Rows[0].Country != "PL" || all.Rows[2].ID != "d" || all.Rows[2].Country != "" {
		t.Errorf("all: %+v", all.Rows)
	}
	if !strings.Contains(rec.Body.String(), `"name":"Глеб","rating"`) {
		t.Errorf("an empty country should be left out: %s", rec.Body)
	}
	// a country nobody lives in is an empty list, a code that is not on the list is refused
	if rec := get(h, "/api/leaderboard?category=blitz&country=JP"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Errorf("empty country: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{"XX", "by", "BYY", "1", "%3Cb%3E"} {
		if rec := get(h, "/api/leaderboard?category=blitz&country="+bad); rec.Code != 400 {
			t.Errorf("country=%s: %d", bad, rec.Code)
		}
	}
	// the cache keeps a table per country: asking for Belarus does not change the table of everybody
	s := service(m, t0)
	s.ttl = time.Hour
	by, _ := s.Leaderboard(context.Background(), ratings.Blitz, "BY", 50)
	every, _ := s.Leaderboard(context.Background(), ratings.Blitz, "", 50)
	if len(by) != 2 || len(every) != 4 {
		t.Errorf("cached tables mixed up: %d and %d rows", len(by), len(every))
	}
}

func TestProfileOpenFields(t *testing.T) {
	m := newMem()
	m.add("a", "Анна", ratings.Blitz, 1800, 50, 20)
	m.users["a"].FirstName, m.users["a"].LastName = "Анна", "Иванова"
	m.users["a"].Location, m.users["a"].About = "Минск", "Люблю <b>эндшпили</b>.\nИграю по вечерам."
	m.users["a"].Country, m.users["a"].AvatarURL = "BY", "preset:crown"
	m.add("b", "Борис", ratings.Blitz, 1600, 50, 20)
	m.users["b"].FirstName = "Борис"
	m.users["b"].Country = "ZZ" // a code the list does not know is not shown
	h := service(m, t0).Handler()

	var p Profile
	rec := get(h, "/api/users/a")
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.FullName != "Анна Иванова" || p.FirstName != "Анна" || p.LastName != "Иванова" || p.Location != "Минск" ||
		p.Country != "BY" || p.CountryName != "Беларусь" || p.AvatarURL != "preset:crown" ||
		p.About != "Люблю <b>эндшпили</b>.\nИграю по вечерам." {
		t.Errorf("profile: %+v", p)
	}
	if low := strings.ToLower(rec.Body.String()); strings.Contains(low, "email") || strings.Contains(low, "settings") || strings.Contains(low, "googleavatar") {
		t.Errorf("private field in %s", rec.Body)
	}

	p = Profile{}
	rec = get(h, "/api/users/b")
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.FullName != "Борис" || p.Country != "" || p.CountryName != "" {
		t.Errorf("one name, unknown country: %+v", p)
	}
	for _, key := range []string{"lastName", "location", "about", "country"} {
		if strings.Contains(rec.Body.String(), `"`+key+`"`) {
			t.Errorf("empty %s should be left out: %s", key, rec.Body)
		}
	}
}

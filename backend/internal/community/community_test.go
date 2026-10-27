package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/store"
)

// The tests need a real MySQL (they are the tests of the queries too): point DYANIS_TEST_DSN at one.
type env struct {
	t      *testing.T
	st     *store.Store
	svc    *Service
	h      http.Handler
	now    time.Time
	nowMu  sync.Mutex
	rating map[string]int
	games  []startedGame

	accounts *sync.Map // id -> *store.User: who a request is from (the X-Test-User header)
}

type startedGame struct {
	white, black play.Player
	tc           play.TimeControl
	rated        bool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("DYANIS_TEST_DSN")
	if dsn == "" {
		t.Skip("DYANIS_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dsn, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	e := &env{t: t, st: st, now: time.Now().UTC().Truncate(time.Millisecond), rating: map[string]int{}}
	var accounts sync.Map // test user header -> *store.User
	e.svc = New(Config{
		Store: st,
		User: func(r *http.Request) *store.User {
			if v, ok := accounts.Load(r.Header.Get("X-Test-User")); ok {
				return v.(*store.User)
			}
			return nil
		},
		Start: func(ctx context.Context, white, black play.Player, tc play.TimeControl, rated bool) (string, error) {
			e.games = append(e.games, startedGame{white, black, tc, rated})
			return fmt.Sprintf("game%012d", len(e.games)), nil
		},
		Rating: func(ctx context.Context, id, cat string) (ratings.Entry, error) {
			r := 1200
			if v, ok := e.rating[id]; ok {
				r = v
			}
			return ratings.Entry{Category: cat, Rating: r}, nil
		},
		Now: func() time.Time { e.nowMu.Lock(); defer e.nowMu.Unlock(); return e.now },
	})
	e.h = e.svc.Handler()
	e.accounts = &accounts
	return e
}

func (e *env) advance(d time.Duration) { e.nowMu.Lock(); e.now = e.now.Add(d); e.nowMu.Unlock() }

// user makes an account and returns it; its settings is the JSON given ("" for none).
func (e *env) user(name, settings string) *store.User {
	e.t.Helper()
	u := &store.User{ID: fmt.Sprintf("cm%014d", time.Now().UnixNano()%1e14), Kind: store.KindGuest, Name: name}
	if err := e.st.CreateUser(context.Background(), u); err != nil {
		e.t.Fatal(err)
	}
	if settings != "" {
		if err := e.st.SetSettings(context.Background(), u.ID, []byte(settings)); err != nil {
			e.t.Fatal(err)
		}
		u.Settings = []byte(settings)
	}
	e.accounts.Store(u.ID, u)
	e.t.Cleanup(func() { _ = e.st.DeleteUser(context.Background(), u.ID) })
	time.Sleep(2 * time.Millisecond) // ids come from the clock
	return u
}

// call sends a request as a user and decodes the JSON answer into out (when not nil).
func (e *env) call(u *store.User, method, path string, body any, out any) int {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if u != nil {
		req.Header.Set("X-Test-User", u.ID)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			e.t.Fatalf("%s %s: bad JSON %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func (e *env) errorOf(u *store.User, method, path string, body any) (int, string) {
	var m map[string]any
	code := e.call(u, method, path, body, &m)
	msg, _ := m["error"].(string)
	return code, msg
}

func (e *env) befriend(a, b *store.User) {
	e.t.Helper()
	if code := e.call(a, "POST", "/api/community/friends", map[string]string{"to": b.ID}, nil); code != 201 {
		e.t.Fatalf("request: %d", code)
	}
	if code := e.call(b, "POST", "/api/community/friends/"+a.ID+"/accept", nil, nil); code != 200 {
		e.t.Fatalf("accept: %d", code)
	}
}

func TestFriendRequests(t *testing.T) {
	e := newEnv(t)
	a, b, c := e.user("Аня", ""), e.user("Борис", ""), e.user("Вера", "")

	if code := e.call(nil, "GET", "/api/community/friends", nil, nil); code != 401 {
		t.Errorf("nobody signed in: %d", code)
	}
	if code, _ := e.errorOf(a, "POST", "/api/community/friends", map[string]string{"to": a.ID}); code != 400 {
		t.Errorf("a friend request to oneself: %d", code)
	}
	if code, _ := e.errorOf(a, "POST", "/api/community/friends", map[string]string{"to": "nobody"}); code != 404 {
		t.Errorf("to a stranger that is not there: %d", code)
	}

	var st map[string]string
	if code := e.call(a, "POST", "/api/community/friends", map[string]string{"to": b.ID}, &st); code != 201 || st["status"] != "requested" {
		t.Fatalf("request: %d %v", code, st)
	}
	if code, msg := e.errorOf(a, "POST", "/api/community/friends", map[string]string{"to": b.ID}); code != 409 {
		t.Errorf("the same request twice: %d %s", code, msg)
	}

	var list struct {
		Friends  []FriendJSON  `json:"friends"`
		Incoming []RequestJSON `json:"incoming"`
		Outgoing []RequestJSON `json:"outgoing"`
	}
	e.call(b, "GET", "/api/community/friends", nil, &list)
	if len(list.Incoming) != 1 || list.Incoming[0].ID != a.ID || len(list.Friends) != 0 {
		t.Fatalf("b sees %+v", list)
	}
	e.call(a, "GET", "/api/community/friends", nil, &list)
	if len(list.Outgoing) != 1 || list.Outgoing[0].ID != b.ID {
		t.Fatalf("a sees %+v", list)
	}
	var sum Summary
	e.call(b, "GET", "/api/community/summary", nil, &sum)
	if sum.FriendRequests != 1 {
		t.Errorf("summary: %+v", sum)
	}

	if code := e.call(b, "POST", "/api/community/friends/"+a.ID+"/accept", nil, &st); code != 200 || st["status"] != "friends" {
		t.Fatalf("accept: %d %v", code, st)
	}
	for _, p := range [][2]*store.User{{a, b}, {b, a}} {
		e.call(p[0], "GET", "/api/community/friends", nil, &list)
		if len(list.Friends) != 1 || list.Friends[0].ID != p[1].ID || len(list.Incoming)+len(list.Outgoing) != 0 {
			t.Errorf("%s sees %+v", p[0].Name, list)
		}
		if !list.Friends[0].Online {
			t.Errorf("a friend that has just asked is online: %+v", list.Friends[0])
		}
	}
	if code, _ := e.errorOf(b, "POST", "/api/community/friends", map[string]string{"to": a.ID}); code != 409 {
		t.Errorf("a request between friends: %d", code)
	}
	if code, _ := e.errorOf(b, "POST", "/api/community/friends/"+a.ID+"/accept", nil); code != 404 {
		t.Errorf("accepting twice: %d", code)
	}

	// Two people who ask each other become friends at once.
	e.call(a, "POST", "/api/community/friends", map[string]string{"to": c.ID}, nil)
	if code := e.call(c, "POST", "/api/community/friends", map[string]string{"to": a.ID}, &st); code != 200 || st["status"] != "friends" {
		t.Errorf("crossed requests: %d %v", code, st)
	}

	// A request can be declined and taken back.
	d := e.user("Глеб", "")
	e.call(d, "POST", "/api/community/friends", map[string]string{"to": b.ID}, nil)
	e.call(b, "POST", "/api/community/friends/"+d.ID+"/decline", nil, nil)
	e.call(b, "GET", "/api/community/friends", nil, &list)
	if len(list.Incoming) != 0 {
		t.Errorf("declined request is still there: %+v", list.Incoming)
	}
	e.call(d, "POST", "/api/community/friends", map[string]string{"to": b.ID}, nil)
	e.call(d, "DELETE", "/api/community/friends/"+b.ID, nil, nil)
	e.call(b, "GET", "/api/community/friends", nil, &list)
	if len(list.Incoming) != 0 {
		t.Errorf("taken back request is still there: %+v", list.Incoming)
	}

	// Ending a friendship works from either side.
	e.call(b, "DELETE", "/api/community/friends/"+a.ID, nil, nil)
	var rel map[string]any
	e.call(a, "GET", "/api/community/friends/"+b.ID, nil, &rel)
	if rel["relation"] != "none" {
		t.Errorf("after ending: %v", rel)
	}

	// Search finds by the beginning of the name and never the person themselves.
	var found struct{ Items []Person }
	e.call(a, "GET", "/api/community/search?q=%D0%91%D0%BE%D1%80", nil, &found)
	hit := false
	for _, p := range found.Items {
		hit = hit || p.ID == b.ID
		if p.ID == a.ID {
			t.Error("search returned the searcher")
		}
	}
	if !hit {
		t.Errorf("search: %+v", found.Items)
	}
}

func TestMessagesOnlyBetweenFriends(t *testing.T) {
	e := newEnv(t)
	a, b, s := e.user("Аня", ""), e.user("Борис", ""), e.user("Сосед", "")
	e.befriend(a, b)

	if code, _ := e.errorOf(s, "POST", "/api/community/messages/"+a.ID, map[string]string{"text": "привет"}); code != 403 {
		t.Errorf("a stranger writes: %d", code)
	}
	if code, _ := e.errorOf(s, "GET", "/api/community/messages/"+a.ID, nil); code != 403 {
		t.Errorf("a stranger reads: %d", code)
	}
	if code, _ := e.errorOf(a, "POST", "/api/community/messages/"+b.ID, map[string]string{"text": "  \n  "}); code != 400 {
		t.Errorf("an empty message: %d", code)
	}

	var sent MessageJSON
	if code := e.call(a, "POST", "/api/community/messages/"+b.ID, map[string]string{"text": "Сыграем?\x00\x07 <b>да</b>"}, &sent); code != 201 || !sent.Mine {
		t.Fatalf("send: %d %+v", code, sent)
	}
	if strings.ContainsAny(sent.Text, "\x00\x07") || !strings.Contains(sent.Text, "<b>да</b>") {
		t.Errorf("control characters go, text stays text: %q", sent.Text)
	}
	e.call(a, "POST", "/api/community/messages/"+b.ID, map[string]string{"text": strings.Repeat("я", 3000)}, &sent)
	if n := len([]rune(sent.Text)); n != MaxMessageRunes {
		t.Errorf("a long message is cut to %d, got %d", MaxMessageRunes, n)
	}

	var sum Summary
	e.call(b, "GET", "/api/community/summary", nil, &sum)
	if sum.Messages != 2 {
		t.Errorf("unread: %+v", sum)
	}
	var convs struct{ Items []ConversationJSON }
	e.call(b, "GET", "/api/community/conversations", nil, &convs)
	if len(convs.Items) != 1 || convs.Items[0].ID != a.ID || convs.Items[0].Unread != 2 || convs.Items[0].Mine {
		t.Errorf("conversations: %+v", convs.Items)
	}

	var talk struct{ Messages []MessageJSON }
	e.call(b, "GET", "/api/community/messages/"+a.ID, nil, &talk)
	if len(talk.Messages) != 2 || talk.Messages[0].Mine || !strings.HasPrefix(talk.Messages[0].Text, "Сыграем") {
		t.Fatalf("the talk: %d messages, first %+v", len(talk.Messages), talk.Messages[0])
	}
	e.call(b, "GET", "/api/community/summary", nil, &sum)
	if sum.Messages != 0 {
		t.Errorf("reading marks as read: %+v", sum)
	}
	e.call(a, "GET", "/api/community/messages/"+b.ID, nil, &talk)
	if !talk.Messages[0].Read {
		t.Errorf("the sender sees it was read: %+v", talk.Messages[0])
	}

	// Only what is new.
	first := talk.Messages[0].ID
	e.call(b, "POST", "/api/community/messages/"+a.ID, map[string]string{"text": "Давай"}, nil)
	e.call(a, "GET", fmt.Sprintf("/api/community/messages/%s?after=%d", b.ID, first+1), nil, &talk)
	if len(talk.Messages) != 1 || talk.Messages[0].Text != "Давай" {
		t.Errorf("after: %+v", talk.Messages)
	}

	// Not too fast.
	limited := false
	for i := 0; i < 30; i++ {
		if code := e.call(a, "POST", "/api/community/messages/"+b.ID, map[string]string{"text": "спам"}, nil); code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("messages are not limited")
	}

	// After the friendship ends the talk stays but is closed.
	e.call(a, "DELETE", "/api/community/friends/"+b.ID, nil, nil)
	if code, _ := e.errorOf(b, "POST", "/api/community/messages/"+a.ID, map[string]string{"text": "ещё"}); code != 403 {
		t.Errorf("writing after the end of friendship: %d", code)
	}
}

func (e *env) challengeBody(extra map[string]any) map[string]any {
	m := map[string]any{"color": "white", "baseMs": 300000, "incMs": 2000}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestChallengeRules(t *testing.T) {
	e := newEnv(t)
	a := e.user("Аня", "")
	b := e.user("Борис", "") // the default: challenges from friends only
	body := func(to string, extra map[string]any) map[string]any {
		m := e.challengeBody(extra)
		m["to"] = to
		return m
	}

	if code, msg := e.errorOf(a, "POST", "/api/community/challenges", body(b.ID, nil)); code != 403 || !strings.Contains(msg, "только от друзей") {
		t.Fatalf("a stranger challenges a friends-only player: %d %s", code, msg)
	}
	e.befriend(a, b)
	var made struct{ ID string }
	if code := e.call(a, "POST", "/api/community/challenges", body(b.ID, nil), &made); code != 201 || made.ID == "" {
		t.Fatalf("a friend challenges: %d", code)
	}
	if code, _ := e.errorOf(a, "POST", "/api/community/challenges", body(b.ID, nil)); code != 409 {
		t.Errorf("a second open challenge to the same person: %d", code)
	}

	var lists struct{ Incoming, Outgoing []ChallengeJSON }
	e.call(b, "GET", "/api/community/challenges", nil, &lists)
	if len(lists.Incoming) != 1 || lists.Incoming[0].Person.ID != a.ID || lists.Incoming[0].BaseMs != 300000 || lists.Incoming[0].Rating != 1200 || !lists.Incoming[0].Friend {
		t.Fatalf("incoming: %+v", lists.Incoming)
	}
	e.call(a, "GET", "/api/community/challenges", nil, &lists)
	if len(lists.Outgoing) != 1 || lists.Outgoing[0].Person.ID != b.ID || lists.Outgoing[0].Status != "pending" {
		t.Fatalf("outgoing: %+v", lists.Outgoing)
	}
	var sum Summary
	e.call(b, "GET", "/api/community/summary", nil, &sum)
	if sum.Challenges != 1 {
		t.Errorf("summary: %+v", sum)
	}

	// Only the addressee accepts; the first answer wins.
	if code, _ := e.errorOf(a, "POST", "/api/community/challenges/"+made.ID+"/accept", nil); code != 404 {
		t.Errorf("the sender accepts: %d", code)
	}
	var game map[string]string
	if code := e.call(b, "POST", "/api/community/challenges/"+made.ID+"/accept", nil, &game); code != 200 || game["gameId"] == "" {
		t.Fatalf("accept: %d %v", code, game)
	}
	if code, _ := e.errorOf(b, "POST", "/api/community/challenges/"+made.ID+"/accept", nil); code != 409 {
		t.Errorf("accepting twice: %d", code)
	}
	if len(e.games) != 1 || e.games[0].white.ID != a.ID || e.games[0].black.ID != b.ID || e.games[0].tc.BaseMs != 300000 || e.games[0].tc.IncMs != 2000 || e.games[0].rated {
		t.Fatalf("the game: %+v", e.games)
	}
	e.call(a, "GET", "/api/community/summary", nil, &sum)
	if len(sum.Accepted) != 1 || sum.Accepted[0].GameID != game["gameId"] {
		t.Errorf("the sender is told: %+v", sum)
	}

	// A muted friend cannot challenge; the other way is not affected.
	if code := e.call(b, "PUT", "/api/community/friends/"+a.ID+"/mute", map[string]bool{"muted": true}, nil); code != 200 {
		t.Fatalf("mute: %d", code)
	}
	if code, msg := e.errorOf(a, "POST", "/api/community/challenges", body(b.ID, nil)); code != 403 || !strings.Contains(msg, "не принимает") {
		t.Errorf("a muted friend: %d %s", code, msg)
	}
	if code := e.call(b, "POST", "/api/community/challenges", body(a.ID, nil), nil); code != 201 {
		t.Errorf("the muting side may still challenge: %d", code)
	}
	var fl struct{ Friends []FriendJSON }
	e.call(b, "GET", "/api/community/friends", nil, &fl)
	if len(fl.Friends) != 1 || !fl.Friends[0].Muted {
		t.Errorf("mute shows in the list: %+v", fl.Friends)
	}
	e.call(b, "PUT", "/api/community/friends/"+a.ID+"/mute", map[string]bool{"muted": false}, nil)
	e.call(a, "GET", "/api/community/challenges", nil, &lists) // cancel what is open from b to a so that the limits stay out of the way
	for _, c := range lists.Incoming {
		e.call(a, "POST", "/api/community/challenges/"+c.ID+"/decline", nil, nil)
	}
	if code := e.call(a, "POST", "/api/community/challenges", body(b.ID, nil), nil); code != 201 {
		t.Errorf("after unmuting: %d", code)
	}
}

func TestChallengeSettingsOfTheAddressee(t *testing.T) {
	e := newEnv(t)
	a := e.user("Аня", "")
	body := func(to string, extra map[string]any) map[string]any {
		m := e.challengeBody(extra)
		m["to"] = to
		return m
	}
	nobody := e.user("Никто", `{"challengesFrom":"none"}`)
	everyone := e.user("Все", `{"challengesFrom":"all"}`)
	ranged := e.user("Диапазон", `{"challengesFrom":"all","challengeMin":1500,"challengeMax":1800}`)
	rangedFriendsFree := e.user("Друзьям можно", `{"challengesFrom":"all","challengeMin":1500,"challengeFriendsIgnoreRating":true}`)
	away := e.user("Не дома", `{"challengesFrom":"all","challengeOffline":false}`)

	if code, _ := e.errorOf(a, "POST", "/api/community/challenges", body(nobody.ID, nil)); code != 403 {
		t.Errorf("nobody: %d", code)
	}
	if code := e.call(a, "POST", "/api/community/challenges", body(everyone.ID, nil), nil); code != 201 {
		t.Errorf("everyone: %d", code)
	}

	if code, msg := e.errorOf(a, "POST", "/api/community/challenges", body(ranged.ID, nil)); code != 403 || !strings.Contains(msg, "рейтинг") {
		t.Errorf("rating 1200 under the lower limit 1500: %d %s", code, msg)
	}
	e.rating[a.ID] = 1600
	if code := e.call(a, "POST", "/api/community/challenges", body(ranged.ID, nil), nil); code != 201 {
		t.Errorf("rating 1600 inside the range: %d", code)
	}
	e.rating[a.ID] = 2400
	e.advance(time.Second)
	if code, _ := e.errorOf(a, "POST", "/api/community/challenges", body(rangedFriendsFree.ID, map[string]any{"days": 3, "baseMs": 0, "incMs": 0})); code != 201 {
		t.Errorf("a correspondence game uses the rapid rating: no upper limit here: %d", code)
	}
	e.rating[a.ID] = 900
	nope := e.user("Строгий", `{"challengesFrom":"all","challengeMin":1500,"challengeFriendsIgnoreRating":true}`)
	if code, _ := e.errorOf(a, "POST", "/api/community/challenges", body(nope.ID, nil)); code != 403 {
		t.Errorf("below the range, not a friend: %d", code)
	}
	e.befriend(a, nope)
	if code := e.call(a, "POST", "/api/community/challenges", body(nope.ID, nil), nil); code != 201 {
		t.Errorf("below the range, but a friend and friends are let through: %d", code)
	}

	// Away: the addressee must have been seen lately.
	if code, msg := e.errorOf(a, "POST", "/api/community/challenges", body(away.ID, nil)); code != 409 || !strings.Contains(msg, "не в сети") {
		t.Errorf("away: %d %s", code, msg)
	}
	e.call(away, "GET", "/api/community/summary", nil, nil) // comes back
	if code := e.call(a, "POST", "/api/community/challenges", body(away.ID, nil), nil); code != 201 {
		t.Errorf("back: %d", code)
	}
	e.advance(5 * time.Minute)
	e.rating[a.ID] = 1200
	e2 := e.user("Другой", "")
	_ = e2

	// Bad requests.
	for name, extra := range map[string]map[string]any{
		"colour":               {"color": "green"},
		"clock too short":      {"baseMs": 100},
		"rated needs a clock":  {"baseMs": 0, "incMs": 0, "rated": true},
		"rated correspondence": {"baseMs": 0, "incMs": 0, "days": 3, "rated": true},
		"days with a clock":    {"days": 3},
		"days out of the list": {"baseMs": 0, "incMs": 0, "days": 4},
	} {
		if code, _ := e.errorOf(a, "POST", "/api/community/challenges", body(everyone.ID, extra)); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
}

func TestChallengeColoursAndAnswers(t *testing.T) {
	e := newEnv(t)
	a, b := e.user("Аня", `{"challengesFrom":"all"}`), e.user("Борис", `{"challengesFrom":"all"}`)
	send := func(from, to *store.User, color string, extra map[string]any) string {
		m := e.challengeBody(extra)
		m["to"], m["color"] = to.ID, color
		var made struct{ ID string }
		if code := e.call(from, "POST", "/api/community/challenges", m, &made); code != 201 {
			t.Fatalf("send: %d", code)
		}
		return made.ID
	}

	id := send(a, b, "black", map[string]any{"rated": true})
	e.call(b, "POST", "/api/community/challenges/"+id+"/accept", nil, nil)
	if g := e.games[0]; g.white.ID != b.ID || g.black.ID != a.ID || !g.rated {
		t.Errorf("the sender wanted Black: %+v", g)
	}

	id = send(b, a, "white", map[string]any{"baseMs": 0, "incMs": 0, "days": 5})
	e.call(a, "POST", "/api/community/challenges/"+id+"/accept", nil, nil)
	if g := e.games[1]; g.white.ID != b.ID || g.tc.Days != 5 || g.rated {
		t.Errorf("a correspondence game: %+v", g)
	}

	// Declined and taken back challenges cannot be accepted.
	id = send(a, b, "random", nil)
	var st map[string]string
	if code := e.call(b, "POST", "/api/community/challenges/"+id+"/decline", nil, &st); code != 200 || st["status"] != "declined" {
		t.Fatalf("decline: %d %v", code, st)
	}
	if code, _ := e.errorOf(b, "POST", "/api/community/challenges/"+id+"/accept", nil); code != 409 {
		t.Errorf("accepting a declined challenge: %d", code)
	}
	var lists struct{ Outgoing []ChallengeJSON }
	e.call(a, "GET", "/api/community/challenges", nil, &lists)
	declined := false
	for _, c := range lists.Outgoing {
		declined = declined || (c.ID == id && c.Status == "declined")
	}
	if !declined {
		t.Errorf("the sender sees the refusal: %+v", lists.Outgoing)
	}
	id = send(a, b, "random", nil)
	if code, _ := e.errorOf(b, "DELETE", "/api/community/challenges/"+id, nil); code != 404 {
		t.Errorf("the addressee cannot take it back: %d", code)
	}
	if code := e.call(a, "DELETE", "/api/community/challenges/"+id, nil, nil); code != 200 {
		t.Errorf("take back: %d", code)
	}
	if code, _ := e.errorOf(b, "POST", "/api/community/challenges/"+id+"/accept", nil); code != 409 {
		t.Errorf("accepting what was taken back: %d", code)
	}

	// An old challenge runs out.
	id = send(a, b, "random", nil)
	e.advance(3 * time.Hour)
	if code, _ := e.errorOf(b, "POST", "/api/community/challenges/"+id+"/accept", nil); code != 409 {
		t.Errorf("an expired challenge: %d", code)
	}
	var sum Summary
	e.call(b, "GET", "/api/community/summary", nil, &sum)
	if sum.Challenges != 0 {
		t.Errorf("an expired challenge is not counted: %+v", sum)
	}
	e.svc.Sweep()
}

func TestVacation(t *testing.T) {
	e := newEnv(t)
	a := e.user("Аня", `{"corrAutoClaim":false}`)

	if p := e.svc.PlayPrefs(a.ID); p.AutoClaim || !p.Vacation.IsZero() {
		t.Errorf("prefs: %+v", p)
	}
	if code, _ := e.errorOf(a, "PUT", "/api/community/vacation", map[string]int{"days": 30}); code != 400 {
		t.Errorf("too long: %d", code)
	}
	var v map[string]int64
	if code := e.call(a, "PUT", "/api/community/vacation", map[string]int{"days": 7}, &v); code != 200 || v["vacation"] == 0 {
		t.Fatalf("start: %d %v", code, v)
	}
	if p := e.svc.PlayPrefs(a.ID); p.Vacation.IsZero() {
		t.Errorf("the vacation reaches the games: %+v", p)
	}
	var sum Summary
	e.call(a, "GET", "/api/community/summary", nil, &sum)
	if sum.Vacation == 0 {
		t.Errorf("summary: %+v", sum)
	}
	if code, _ := e.errorOf(a, "PUT", "/api/community/vacation", map[string]int{"days": 3}); code != 409 {
		t.Errorf("a vacation within a vacation: %d", code)
	}
	if code := e.call(a, "PUT", "/api/community/vacation", map[string]int{"days": 0}, &v); code != 200 || v["vacation"] != 0 {
		t.Fatalf("end: %d %v", code, v)
	}
	if p := e.svc.PlayPrefs(a.ID); !p.Vacation.IsZero() {
		t.Errorf("ended: %+v", p)
	}
	if code, msg := e.errorOf(a, "PUT", "/api/community/vacation", map[string]int{"days": 3}); code != 409 || !strings.Contains(msg, "14 дней") {
		t.Errorf("a new vacation at once: %d %s", code, msg)
	}
	e.advance(15 * 24 * time.Hour)
	if code := e.call(a, "PUT", "/api/community/vacation", map[string]int{"days": 3}, nil); code != 200 {
		t.Errorf("after two weeks: %d", code)
	}
}

func TestParsePrefs(t *testing.T) {
	def := ParsePrefs(nil)
	if def.ChallengesFrom != "friends" || !def.FriendsIgnore || !def.Offline || !def.AutoClaim || def.Min != 0 || def.Max != 0 {
		t.Errorf("defaults: %+v", def)
	}
	if p := ParsePrefs([]byte(`not json`)); p != def {
		t.Errorf("garbage: %+v", p)
	}
	if p := ParsePrefs([]byte(`{"challengesFrom":"everybody","challengeMin":-5,"challengeMax":99999,"corrAutoClaim":"yes"}`)); p != def {
		t.Errorf("wrong values fall back: %+v", p)
	}
	p := ParsePrefs([]byte(`{"challengesFrom":"none","challengeMin":800,"challengeMax":2000,"challengeFriendsIgnoreRating":false,"challengeOffline":false,"corrAutoClaim":false,"boardTheme":"x"}`))
	if p != (Prefs{ChallengesFrom: "none", Min: 800, Max: 2000, FriendsIgnore: false, Offline: false, AutoClaim: false}) {
		t.Errorf("all keys: %+v", p)
	}
}

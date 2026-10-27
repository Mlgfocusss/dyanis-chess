package community

import (
	"strings"
	"testing"
	"time"
)

func TestBlockClosesEveryDoor(t *testing.T) {
	e := newEnv(t)
	a, b := e.user("Аня-блок", `{"challengesFrom":"all"}`), e.user("Борис-блок", `{"challengesFrom":"all"}`)
	c, d := e.user("Вера-блок", ""), e.user("Глеб-блок", "")
	e.befriend(a, b)
	e.call(a, "POST", "/api/community/messages/"+b.ID, map[string]string{"text": "привет"}, nil)
	e.call(b, "POST", "/api/community/messages/"+a.ID, map[string]string{"text": "и тебе"}, nil)
	e.call(c, "POST", "/api/community/friends", map[string]string{"to": a.ID}, nil) // a request to a
	e.call(a, "POST", "/api/community/friends", map[string]string{"to": d.ID}, nil) // a request from a
	var made struct{ ID string }
	body := func(to string) map[string]any {
		m := e.challengeBody(nil)
		m["to"] = to
		return m
	}
	if code := e.call(b, "POST", "/api/community/challenges", body(a.ID), &made); code != 201 { // b -> a, open
		t.Fatalf("challenge: %d", code)
	}
	var sum Summary
	e.call(a, "GET", "/api/community/summary", nil, &sum)
	if sum.Challenges != 1 || sum.Messages != 1 {
		t.Fatalf("before the block: %+v", sum)
	}

	if code, _ := e.errorOf(a, "PUT", "/api/community/blocks/"+a.ID, nil); code != 400 {
		t.Errorf("block oneself: %d", code)
	}
	if code, _ := e.errorOf(a, "PUT", "/api/community/blocks/nobody", nil); code != 404 {
		t.Errorf("block a stranger that is not there: %d", code)
	}
	if code, _ := e.errorOf(nil, "PUT", "/api/community/blocks/"+b.ID, nil); code != 401 {
		t.Errorf("block without signing in: %d", code)
	}
	var st map[string]string
	if code := e.call(a, "PUT", "/api/community/blocks/"+b.ID, nil, &st); code != 200 || st["status"] != "blocked" {
		t.Fatalf("block: %d %v", code, st)
	}
	if code := e.call(a, "PUT", "/api/community/blocks/"+b.ID, nil, nil); code != 200 {
		t.Errorf("blocking twice is the same: %d", code)
	}

	// The ties are gone: no friendship, no requests, no open challenge, no unread badge.
	var list struct {
		Friends  []FriendJSON  `json:"friends"`
		Incoming []RequestJSON `json:"incoming"`
		Outgoing []RequestJSON `json:"outgoing"`
	}
	for _, u := range []string{a.ID, b.ID} {
		who := a
		if u == b.ID {
			who = b
		}
		e.call(who, "GET", "/api/community/friends", nil, &list)
		if len(list.Friends) != 0 {
			t.Errorf("%s still has friends: %+v", who.Name, list.Friends)
		}
	}
	e.call(a, "GET", "/api/community/friends", nil, &list)
	if len(list.Incoming) != 1 || list.Incoming[0].ID != c.ID || len(list.Outgoing) != 1 {
		t.Errorf("the requests of others are not touched: %+v", list)
	}
	e.call(a, "GET", "/api/community/summary", nil, &sum)
	if sum.Challenges != 0 || sum.Messages != 0 {
		t.Errorf("after the block: %+v", sum)
	}
	if code, _ := e.errorOf(a, "POST", "/api/community/challenges/"+made.ID+"/accept", nil); code != 409 {
		t.Errorf("accepting a challenge of a blocked person: %d", code)
	}
	if len(e.games) != 0 {
		t.Errorf("a game was made: %+v", e.games)
	}

	// The blocked person gets neutral refusals.
	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"message", "POST", "/api/community/messages/" + a.ID, map[string]string{"text": "эй"}},
		{"friend request", "POST", "/api/community/friends", map[string]string{"to": a.ID}},
		{"challenge", "POST", "/api/community/challenges", body(a.ID)},
	} {
		code, msg := e.errorOf(b, tc.method, tc.path, tc.body)
		if code != 403 || strings.Contains(strings.ToLower(msg), "заблок") {
			t.Errorf("blocked person, %s: %d %q", tc.name, code, msg)
		}
	}
	// The one who blocked cannot do it either until they unblock.
	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"message", "POST", "/api/community/messages/" + b.ID, map[string]string{"text": "эй"}},
		{"friend request", "POST", "/api/community/friends", map[string]string{"to": b.ID}},
		{"challenge", "POST", "/api/community/challenges", body(b.ID)},
	} {
		code, msg := e.errorOf(a, tc.method, tc.path, tc.body)
		if code != 403 || !strings.Contains(msg, "разблокируйте") {
			t.Errorf("blocker, %s: %d %q", tc.name, code, msg)
		}
	}

	// What each side sees on the page of the other.
	var rel map[string]any
	e.call(a, "GET", "/api/community/friends/"+b.ID, nil, &rel)
	if rel["relation"] != "blocked" {
		t.Errorf("blocker's relation: %v", rel)
	}
	e.call(b, "GET", "/api/community/friends/"+a.ID, nil, &rel)
	if rel["relation"] != "none" {
		t.Errorf("blocked person's relation: %v", rel)
	}

	// The list of the blocker, and nothing of it for the blocked.
	var blocks struct {
		Items []BlockedJSON
		Max   int
	}
	e.call(a, "GET", "/api/community/blocks", nil, &blocks)
	if len(blocks.Items) != 1 || blocks.Items[0].ID != b.ID || blocks.Max != MaxBlocks {
		t.Errorf("blocks: %+v", blocks)
	}
	e.call(b, "GET", "/api/community/blocks", nil, &blocks)
	if len(blocks.Items) != 0 {
		t.Errorf("the blocked person's own list: %+v", blocks)
	}

	// Presence: the blocked person does not see the other as being on the site, in the talks they still have.
	var convs struct{ Items []ConversationJSON }
	e.call(a, "GET", "/api/community/summary", nil, nil) // a is here now
	e.call(b, "GET", "/api/community/conversations", nil, &convs)
	if len(convs.Items) != 1 || convs.Items[0].ID != a.ID || convs.Items[0].Online {
		t.Errorf("the blocked person's talks: %+v", convs.Items)
	}
	e.call(a, "GET", "/api/community/conversations", nil, &convs)
	if len(convs.Items) != 1 || convs.Items[0].ID != b.ID || !convs.Items[0].Online {
		t.Errorf("the blocker's talks: %+v", convs.Items)
	}

	// Search: the blocked person does not find the blocker; the blocker finds anybody.
	var found struct{ Items []Person }
	e.call(b, "GET", "/api/community/search?q="+"%D0%90%D0%BD%D1%8F-%D0%B1%D0%BB", nil, &found)
	for _, p := range found.Items {
		if p.ID == a.ID {
			t.Error("the blocked person found the blocker by name")
		}
	}
	e.call(a, "GET", "/api/community/search?q="+"%D0%91%D0%BE%D1%80%D0%B8%D1%81-%D0%B1%D0%BB", nil, &found)
	hit := false
	for _, p := range found.Items {
		hit = hit || p.ID == b.ID
	}
	if !hit {
		t.Error("the blocker does not find the blocked person")
	}

	// Taking the block back opens the doors again (the friendship does not come back by itself).
	if code := e.call(a, "DELETE", "/api/community/blocks/"+b.ID, nil, nil); code != 200 {
		t.Fatalf("unblock: %d", code)
	}
	e.call(a, "GET", "/api/community/blocks", nil, &blocks)
	if len(blocks.Items) != 0 {
		t.Errorf("blocks after unblocking: %+v", blocks)
	}
	if code, _ := e.errorOf(b, "POST", "/api/community/messages/"+a.ID, map[string]string{"text": "эй"}); code != 403 {
		t.Errorf("not friends any more: %d", code) // the old rule: only friends write
	}
	if code := e.call(b, "POST", "/api/community/challenges", body(a.ID), nil); code != 201 {
		t.Errorf("a challenge after unblocking: %d", code)
	}
	if code := e.call(b, "POST", "/api/community/friends", map[string]string{"to": a.ID}, &st); code != 201 {
		t.Errorf("a friend request after unblocking: %d", code)
	}
	if code := e.call(a, "DELETE", "/api/community/blocks/"+b.ID, nil, nil); code != 200 {
		t.Errorf("unblocking twice is the same: %d", code)
	}
}

func TestBlockLimits(t *testing.T) {
	e := newEnv(t)
	a := e.user("Аня-лимит", "")
	people := []string{e.user("П1", "").ID, e.user("П2", "").ID, e.user("П3", "").ID}

	old := maxBlocks
	maxBlocks = 2
	defer func() { maxBlocks = old }()
	for _, id := range people[:2] {
		if code := e.call(a, "PUT", "/api/community/blocks/"+id, nil, nil); code != 200 {
			t.Fatalf("block: %d", code)
		}
	}
	if code, _ := e.errorOf(a, "PUT", "/api/community/blocks/"+people[2], nil); code != 409 {
		t.Errorf("over the number of blocks: %d", code)
	}
	if code := e.call(a, "PUT", "/api/community/blocks/"+people[0], nil, nil); code != 200 {
		t.Errorf("blocking somebody blocked already is not over the limit: %d", code)
	}

	// 30 actions a minute (two were made above, plus the refused one counts too).
	n := 0
	for ; n < 40; n++ {
		if code := e.call(a, "DELETE", "/api/community/blocks/"+people[2], nil, nil); code == 429 {
			break
		}
	}
	if n < 20 || n >= 40 {
		t.Errorf("the rate limit hit after %d actions", n)
	}
	e.advance(61 * time.Second)
	if code := e.call(a, "DELETE", "/api/community/blocks/"+people[2], nil, nil); code != 200 {
		t.Errorf("a minute later: %d", code)
	}
}

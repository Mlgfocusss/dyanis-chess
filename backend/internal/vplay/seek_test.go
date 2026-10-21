package vplay

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func seekCall(t *testing.T, srvCall func(method, body string) (int, []byte), method, body string) SeekStatus {
	t.Helper()
	code, b := srvCall(method, body)
	var st SeekStatus
	if code != 200 || json.Unmarshal(b, &st) != nil {
		t.Fatalf("seek %s: %d %s", method, code, b)
	}
	return st
}

func TestQuickGamePairsTwoPeopleWhoAskedForTheSame(t *testing.T) {
	h, srv := newTestHub(t, &memStore{})
	as := func(user string) func(method, body string) (int, []byte) {
		return func(method, body string) (int, []byte) { return call(t, srv, user, method, "/api/variants/seek", body) }
	}
	const body = `{"variant":"classic","baseMs":180000,"incMs":2000}`

	if st := seekCall(t, as("anna"), "GET", ""); st.State != "none" {
		t.Fatalf("nothing asked yet: %+v", st)
	}
	if st := seekCall(t, as("anna"), "POST", body); st.State != "waiting" || st.Count != 1 {
		t.Fatalf("anna waits: %+v", st)
	}
	// somebody who wants another clock or another variant is not a pair
	if st := seekCall(t, as("carl"), "POST", `{"variant":"classic","baseMs":300000,"incMs":0}`); st.State != "waiting" {
		t.Fatalf("different clock: %+v", st)
	}
	if st := seekCall(t, as("dora"), "POST", `{"variant":"t-opts","baseMs":180000,"incMs":2000}`); st.State != "waiting" {
		t.Fatalf("different variant: %+v", st)
	}
	// the same person asking twice is not their own opponent
	if st := seekCall(t, as("anna"), "POST", body); st.State != "waiting" {
		t.Fatalf("anna with herself: %+v", st)
	}
	bs := seekCall(t, as("boris"), "POST", body)
	if bs.State != "found" || bs.ID == "" {
		t.Fatalf("boris should meet anna: %+v", bs)
	}
	as1 := seekCall(t, as("anna"), "GET", "")
	if as1.State != "found" || as1.ID != bs.ID {
		t.Fatalf("anna is told: %+v", as1)
	}
	// repeating the question gives the same game
	if again := seekCall(t, as("anna"), "GET", ""); again.ID != bs.ID {
		t.Errorf("repeat: %+v", again)
	}

	m := snapshot(t, srv, "anna", bs.ID)
	g := m.Game
	if g.Status != StatusPlaying || g.Variant != "classic" || g.TC.BaseMs != 180000 || g.TC.IncMs != 2000 || g.White == nil || g.Black == nil {
		t.Fatalf("the game: %+v", g)
	}
	ids := map[string]bool{g.White.ID: true, g.Black.ID: true}
	if !ids["anna"] || !ids["boris"] {
		t.Errorf("seated: %s, %s", g.White.ID, g.Black.ID)
	}
	// those who were not paired still wait; a pairing is forgotten after it was collected
	if st := seekCall(t, as("carl"), "GET", ""); st.State != "waiting" {
		t.Errorf("carl: %+v", st)
	}
	_ = h
}

func TestSeekCancelAndExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	h := New(Config{User: testUser, Now: clk.now})
	defer h.Close()
	srv := newServerFor(t, h)
	as := func(user string) func(method, body string) (int, []byte) {
		return func(method, body string) (int, []byte) { return call(t, srv, user, method, "/api/variants/seek", body) }
	}
	const body = `{"variant":"classic","baseMs":60000,"incMs":0}`
	seekCall(t, as("anna"), "POST", body)
	if st := seekCall(t, as("anna"), "DELETE", ""); st.State != "none" {
		t.Fatalf("cancel: %+v", st)
	}
	// nobody to meet any more
	if st := seekCall(t, as("boris"), "POST", body); st.State != "waiting" {
		t.Fatalf("boris: %+v", st)
	}
	// a person who left (no question for 20 s) is dropped, so the next one waits too
	clk.add(25 * time.Second)
	if st := seekCall(t, as("carl"), "POST", body); st.State != "waiting" || st.Count != 1 {
		t.Fatalf("boris should have been dropped: %+v", st)
	}
	// after the pairing is collected and read, it is forgotten
	st := seekCall(t, as("dora"), "POST", body)
	if st.State != "found" {
		t.Fatalf("dora meets carl: %+v", st)
	}
	clk.add(5 * time.Second) // dora learned of it in the answer to her own request
	if st := seekCall(t, as("dora"), "GET", ""); st.State != "found" {
		t.Fatalf("a retry within ten seconds: %+v", st)
	}
	clk.add(6 * time.Second)
	if st := seekCall(t, as("dora"), "GET", ""); st.State != "none" {
		t.Errorf("after the pairing was read and used: %+v", st)
	}
	// cancelling too late returns the game
	if st := seekCall(t, as("carl"), "DELETE", ""); st.State != "found" || st.ID == "" {
		t.Errorf("carl cancels too late: %+v", st)
	}
}

func TestSeekValidation(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	for _, c := range []struct {
		user, body string
		code       int
	}{
		{"", `{"variant":"classic","baseMs":60000}`, http.StatusUnauthorized},
		{"anna", `{"variant":"nope","baseMs":60000}`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","baseMs":5}`, http.StatusBadRequest},
		{"anna", `{"variant":"classic","baseMs":60000,"options":{"z":1}}`, http.StatusBadRequest},
		{"anna", `x`, http.StatusBadRequest},
	} {
		if code, b := call(t, srv, c.user, "POST", "/api/variants/seek", c.body); code != c.code {
			t.Errorf("%s %q: %d %s, want %d", c.user, c.body, code, b, c.code)
		}
	}
	if code, _ := call(t, srv, "", "GET", "/api/variants/seek", ""); code != http.StatusUnauthorized {
		t.Errorf("GET without sign-in: %d", code)
	}
}

// Seekers with the same lobby choices are paired; with different ones they are not.
func TestSeekKeyIncludesOptions(t *testing.T) {
	_, srv := newTestHub(t, &memStore{})
	as := func(user string) func(method, body string) (int, []byte) {
		return func(method, body string) (int, []byte) { return call(t, srv, user, method, "/api/variants/seek", body) }
	}
	seekCall(t, as("anna"), "POST", `{"variant":"t-opts","baseMs":60000,"options":{"n":2}}`)
	if st := seekCall(t, as("boris"), "POST", `{"variant":"t-opts","baseMs":60000,"options":{"n":3}}`); st.State != "waiting" {
		t.Fatalf("other choices: %+v", st)
	}
	st := seekCall(t, as("carl"), "POST", `{"variant":"t-opts","baseMs":60000,"options":{"n":2}}`)
	if st.State != "found" {
		t.Fatalf("same choices: %+v", st)
	}
	m := snapshot(t, srv, "carl", st.ID)
	if m.Game.Params["n"] != float64(2) {
		t.Errorf("the pair's game has %v", m.Game.Params)
	}
}

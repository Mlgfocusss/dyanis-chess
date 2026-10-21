package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"dyanis-chess/backend/internal/store"
)

var jsonHdr = []string{"Content-Type", "application/json"}

func TestValidName(t *testing.T) {
	good := map[string]string{
		"Анна":                  "Анна",
		"  Анна   Иванова ":     "Анна Иванова",
		"Bob_the-2nd.":          "Bob_the-2nd.",
		"Ян":                    "Ян",
		"A B":                   "A B",
		strings.Repeat("я", 24): strings.Repeat("я", 24),
	}
	for in, want := range good {
		if got, err := validName(in); err != nil || got != want {
			t.Errorf("validName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "   ", "a", " я ", strings.Repeat("я", 25), "a\x00b", "a\nb", "a\tb", "Анна<b>", "Аня 🙂", "---", "...", "a/b", "a@b", `a"b`,
	}
	for _, in := range bad {
		if got, err := validName(in); err == nil {
			t.Errorf("validName(%q) = %q, want an error", in, got)
		}
	}
}

func TestProfileRename(t *testing.T) {
	db := newMemStore()
	c := newClient(t, newService(t, db, nil).Handler())

	if rec := c.do("PUT", "/api/me/profile", `{"name":"Анна"}`, jsonHdr...); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	c.do("POST", "/api/auth/guest", "")
	before := c.me().User

	// content type, junk and invalid names change nothing and do not use up the allowance
	if rec := c.do("PUT", "/api/me/profile", `{"name":"Анна"}`, "Content-Type", "text/plain"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form content type: %d", rec.Code)
	}
	for _, body := range []string{`nope`, `{"name":5}`, `{"name":""}`, `{"name":"я"}`, `{"name":"<script>"}`, `{"name":"` + strings.Repeat("я", 30) + `"}`} {
		rec := c.do("PUT", "/api/me/profile", body, jsonHdr...)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, rec.Code)
		}
		var e struct{ Error string }
		json.Unmarshal(rec.Body.Bytes(), &e)
		if e.Error == "" {
			t.Errorf("%s: no message", body)
		}
	}
	if got := c.me().User.Name; got != before.Name {
		t.Fatalf("a rejected request changed the name to %q", got)
	}

	rec := c.do("PUT", "/api/me/profile", `{"name":"  Анна   Иванова "}`, jsonHdr...)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	var r meResp
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil || r.User == nil || r.User.Name != "Анна Иванова" || r.User.ID != before.ID || !r.Google {
		t.Fatalf("response: %s (%v)", rec.Body, err)
	}
	if got := c.me().User.Name; got != "Анна Иванова" {
		t.Errorf("name after rename: %q", got)
	}
	if db.users[before.ID].Name != "Анна Иванова" {
		t.Errorf("stored name: %q", db.users[before.ID].Name)
	}

	// a second change right away is refused, in Russian, with Retry-After
	rec = c.do("PUT", "/api/me/profile", `{"name":"Борис"}`, jsonHdr...)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "раза в минуту") {
		t.Errorf("second change: %d %s (Retry-After %q)", rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}
	if got := c.me().User.Name; got != "Анна Иванова" {
		t.Errorf("a limited request changed the name to %q", got)
	}
	// the same name again is not a change
	if rec := c.do("PUT", "/api/me/profile", `{"name":"Анна Иванова"}`, jsonHdr...); rec.Code != http.StatusOK {
		t.Errorf("same name: %d", rec.Code)
	}

	// another user is not held back by this one's allowance
	c2 := newClient(t, newService(t, db, nil).Handler())
	c2.do("POST", "/api/auth/guest", "")
	if rec := c2.do("PUT", "/api/me/profile", `{"name":"Борис"}`, jsonHdr...); rec.Code != http.StatusOK {
		t.Errorf("other user: %d %s", rec.Code, rec.Body)
	}
}

func TestRenameLimiterReleasedAfterStoreFailure(t *testing.T) {
	l := newNameLimiter(nameChangeEvery)
	if ok, _ := l.allow("u"); !ok {
		t.Fatal("first change refused")
	}
	if ok, wait := l.allow("u"); ok || wait <= 0 || wait > nameChangeEvery {
		t.Errorf("second change: ok=%v wait=%v", ok, wait)
	}
	l.release("u")
	if ok, _ := l.allow("u"); !ok {
		t.Error("released user still limited")
	}
}

func TestDeleteAccount(t *testing.T) {
	db := newMemStore()
	c := newClient(t, newService(t, db, nil).Handler())

	if rec := c.do("DELETE", "/api/me", `{"confirm":"УДАЛИТЬ"}`, jsonHdr...); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	c.do("POST", "/api/auth/guest", "")
	me := c.me().User
	if me == nil {
		t.Fatal("no guest")
	}

	// no confirmation, a wrong one, a form: nothing is deleted
	if rec := c.do("DELETE", "/api/me", "", jsonHdr...); rec.Code != http.StatusBadRequest {
		t.Errorf("no body: %d", rec.Code)
	}
	if rec := c.do("DELETE", "/api/me", `{"confirm":"удалить"}`, jsonHdr...); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong word: %d", rec.Code)
	}
	if rec := c.do("DELETE", "/api/me", `{"confirm":"УДАЛИТЬ"}`, "Content-Type", "text/plain"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form content type: %d", rec.Code)
	}
	if db.users[me.ID] == nil {
		t.Fatal("a refused request deleted the user")
	}

	// an unfinished game with a person: 409 and the account stays
	db.deleteErr = store.ErrActiveGame
	rec := c.do("DELETE", "/api/me", `{"confirm":"УДАЛИТЬ"}`, jsonHdr...)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "партия") {
		t.Errorf("active game: %d %s", rec.Code, rec.Body)
	}
	if c.me().User == nil || db.users[me.ID] == nil {
		t.Fatal("a blocked deletion removed the account")
	}
	db.deleteErr = nil

	rec = c.do("DELETE", "/api/me", `{"confirm":"УДАЛИТЬ"}`, jsonHdr...)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	var cleared bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the session cookie was not cleared")
	}
	if len(db.users) != 0 || len(db.sessions) != 0 {
		t.Errorf("left behind: %d users, %d sessions", len(db.users), len(db.sessions))
	}
	if c.me().User != nil {
		t.Error("still signed in after deletion")
	}
	// an old cookie of the deleted account is worthless
	if rec := c.do("PUT", "/api/me/profile", `{"name":"Анна"}`, jsonHdr...); rec.Code != http.StatusUnauthorized {
		t.Errorf("after deletion: %d", rec.Code)
	}
}

func TestGoogleKeepsAChosenName(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-name", Name: "Имя из Google"})

	// a guest who chose a name keeps it when linking Google
	c := newClient(t, newService(t, db, g).Handler())
	c.do("POST", "/api/auth/guest", "")
	c.do("PUT", "/api/me/profile", `{"name":"Моё имя"}`, jsonHdr...)
	callback(c, startGoogle(t, c), "good-code")
	if u := c.me().User; u.Kind != store.KindGoogle || u.Name != "Моё имя" {
		t.Errorf("after linking: %+v", u)
	}
	// and a later sign-in on another browser does not bring Google's name back
	c2 := newClient(t, newService(t, db, g).Handler())
	callback(c2, startGoogle(t, c2), "good-code")
	if u := c2.me().User; u.Name != "Моё имя" {
		t.Errorf("after signing in again: %+v", u)
	}

	// a guest who kept the generated name gets Google's
	g2 := fakeGoogle(t, profile{Sub: "sub-name-2", Name: "Другое имя"})
	c3 := newClient(t, newService(t, db, g2).Handler())
	c3.do("POST", "/api/auth/guest", "")
	callback(c3, startGoogle(t, c3), "good-code")
	if u := c3.me().User; u.Name != "Другое имя" {
		t.Errorf("generated name was kept: %+v", u)
	}
}

func TestGeneratedGuestNamePattern(t *testing.T) {
	for i := 0; i < 50; i++ {
		if n := guestName(); !generatedGuestName.MatchString(n) {
			t.Fatalf("guestName() = %q does not match the pattern", n)
		}
	}
	if generatedGuestName.MatchString("Гость Иван") || generatedGuestName.MatchString("Гость 12345") {
		t.Error("the pattern is too wide")
	}
}

package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"dyanis-chess/backend/internal/store"
)

func TestValidPersonName(t *testing.T) {
	good := map[string]string{
		"Анна":                  "Анна",
		"  Анна   Мария ":       "Анна Мария",
		"Жан-Люк":               "Жан-Люк",
		"O'Brien":               "O'Brien",
		"D’Artagnan":            "D’Artagnan",
		"Дж.":                   "Дж.",
		"":                      "",
		"   ":                   "",
		strings.Repeat("я", 40): strings.Repeat("я", 40),
		"José":                  "José",
		"Zoë":                   "Zoë",
		"Zoë":                  "Zoë", // a letter with a combining mark
		"Ли":                    "Ли",
		"أحمد":                  "أحمد",
		"Анна Мария":            "Анна Мария",
		strings.Repeat("a", 3):  "aaa",
		"  Анна  ":              "Анна",
		"李":                     "李",
	}
	for in, want := range good {
		if got, err := validPersonName(in, "Имя"); err != nil || got != want {
			t.Errorf("validPersonName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		strings.Repeat("я", 41), "Анна1", "Анна<b>", "a\x00b", "a\nb", "a\tb", "Аня 🙂", "---", "...", "a/b", "a@b", `a"b`, "a_b", "'", "Анна‮Мария",
	}
	for _, in := range bad {
		if got, err := validPersonName(in, "Фамилия"); err == nil {
			t.Errorf("validPersonName(%q) = %q, want an error", in, got)
		} else if !strings.Contains(err.Error(), "Фамилия") {
			t.Errorf("the message should name the field: %v", err)
		}
	}
}

func TestValidLocation(t *testing.T) {
	good := map[string]string{
		"Минск":                 "Минск",
		"  Гродно,   Беларусь ": "Гродно, Беларусь",
		"":                      "",
		"São Paulo":             "São Paulo",
		"Нью-Йорк (США)":        "Нью-Йорк (США)",
		strings.Repeat("я", 60): strings.Repeat("я", 60),
		"Москва Сити":           "Москва Сити",
	}
	for in, want := range good {
		if got, err := validLocation(in); err != nil || got != want {
			t.Errorf("validLocation(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{strings.Repeat("я", 61), "a\nb", "a\tb", "a\x00b", "a\x7fb", "a​b", "a‮b"} {
		if got, err := validLocation(in); err == nil {
			t.Errorf("validLocation(%q) = %q, want an error", in, got)
		}
	}
}

func TestValidAbout(t *testing.T) {
	good := map[string]string{
		"Играю по вечерам.":               "Играю по вечерам.",
		"  привет  \r\nмир \r\n":          "привет\nмир",
		"один\n\n\n\n\nдва":               "один\n\nдва",
		"Тег <b>жирный</b> & \"кавычки\"": "Тег <b>жирный</b> & \"кавычки\"", // plain text, shown as text
		"":                       "",
		" \n \n":                 "",
		strings.Repeat("я", 500): strings.Repeat("я", 500),
		strings.Repeat("я", 249) + "\n\n" + strings.Repeat("я", 249): strings.Repeat("я", 249) + "\n\n" + strings.Repeat("я", 249),
		"строка\rстрока":                                             "строка\nстрока",
	}
	for in, want := range good {
		if got, err := validAbout(in); err != nil || got != want {
			t.Errorf("validAbout(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	// the count is made after tidying: the folded empty lines do not count
	if _, err := validAbout(strings.Repeat("я", 250) + "\n\n\n\n\n\n" + strings.Repeat("я", 250)); err != nil {
		// 250 + "\n\n" + 250 = 502: too long even after folding, so this must fail
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("message: %v", err)
		}
	} else {
		t.Error("502 characters were accepted")
	}
	for _, in := range []string{strings.Repeat("я", 501), "a\tb", "a\x00b", "a\x1bb", "a‮b", "a​b"} {
		if got, err := validAbout(in); err == nil {
			t.Errorf("validAbout(%q) = %q, want an error", in, got)
		}
	}
}

func TestResolveAvatar(t *testing.T) {
	for _, id := range avatarPresets {
		if got, err := resolveAvatar("preset:"+id, ""); err != nil || got != "preset:"+id {
			t.Errorf("preset %s: %q, %v", id, got, err)
		}
	}
	if len(avatarPresets) != 12 {
		t.Errorf("%d presets, want 12", len(avatarPresets))
	}
	if got, err := resolveAvatar("", "https://img/a.png"); err != nil || got != "" {
		t.Errorf("letter: %q, %v", got, err)
	}
	if got, err := resolveAvatar("google", "https://img/a.png"); err != nil || got != "https://img/a.png" {
		t.Errorf("google: %q, %v", got, err)
	}
	if _, err := resolveAvatar("google", ""); err == nil {
		t.Error("the Google photo was given to someone who has none")
	}
	for _, bad := range []string{"preset:", "preset:nope", "preset:crown ", "Preset:crown", "https://evil.test/x.png", "http://evil.test/x.png", "data:image/png;base64,AAAA", "javascript:alert(1)", "GOOGLE", "letter"} {
		if got, err := resolveAvatar(bad, "https://img/a.png"); err == nil {
			t.Errorf("resolveAvatar(%q) = %q, want an error", bad, got)
		}
	}
}

// quick makes saves of the profile fields instant, so a test can save many times; the tests of the
// wait itself keep the real limiter.
func quick(svc *Service) *Service {
	svc.fields = newNameLimiter(0)
	svc.names = newNameLimiter(0)
	return svc
}

func put(c *client, body string) (int, meResp) {
	c.t.Helper()
	rec := c.do("PUT", "/api/me/profile", body, jsonHdr...)
	var r meResp
	json.Unmarshal(rec.Body.Bytes(), &r)
	return rec.Code, r
}

func errorOf(c *client, body string) string {
	c.t.Helper()
	rec := c.do("PUT", "/api/me/profile", body, jsonHdr...)
	if rec.Code != http.StatusBadRequest {
		c.t.Errorf("%s: %d, want 400 (%s)", body, rec.Code, rec.Body)
	}
	var e struct{ Error string }
	json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Error
}

func TestProfileFieldsSaved(t *testing.T) {
	db := newMemStore()
	c := newClient(t, quick(newService(t, db, nil)).Handler())
	c.do("POST", "/api/auth/guest", "")
	id := c.me().User.ID

	code, r := put(c, `{"firstName":"  Анна ","lastName":"Иванова","location":" Минск ","about":"Люблю шахматы.\r\nИграю вечерами.","country":"BY","avatar":"preset:crown"}`)
	if code != 200 || r.User == nil {
		t.Fatalf("save: %d", code)
	}
	want := func(u *userJSON, label string) {
		t.Helper()
		if u.FirstName != "Анна" || u.LastName != "Иванова" || u.Location != "Минск" || u.About != "Люблю шахматы.\nИграю вечерами." ||
			u.Country != "BY" || u.AvatarURL != "preset:crown" {
			t.Errorf("%s: %+v", label, u)
		}
	}
	want(r.User, "response")
	want(c.me().User, "GET /api/me")
	if u := db.users[id]; u.FirstName != "Анна" || u.Country != "BY" || u.AvatarURL != "preset:crown" {
		t.Errorf("stored: %+v", u)
	}

	// only the fields sent change
	if code, r = put(c, `{"location":"Гродно"}`); code != 200 || r.User.Location != "Гродно" || r.User.FirstName != "Анна" || r.User.Country != "BY" || r.User.AvatarURL != "preset:crown" {
		t.Errorf("partial save: %d %+v", code, r.User)
	}
	// the name is optional too, and the display name is not touched by the other fields
	if r.User.Name == "" || !strings.HasPrefix(r.User.Name, "Гость ") {
		t.Errorf("name: %q", r.User.Name)
	}
	// an empty string clears; the country can be cleared
	if code, r = put(c, `{"country":"","about":"","avatar":""}`); code != 200 || r.User.Country != "" || r.User.About != "" || r.User.AvatarURL != "" || r.User.Location != "Гродно" {
		t.Errorf("clearing: %d %+v", code, r.User)
	}
	if u := db.users[id]; u.Country != "" || u.About != "" || u.AvatarURL != "" {
		t.Errorf("stored after clearing: %+v", u)
	}
	// name together with the rest, in one request
	if code, r = put(c, `{"name":"Новое имя","country":"PL"}`); code != 200 || r.User.Name != "Новое имя" || r.User.Country != "PL" {
		t.Errorf("name and country: %d %+v", code, r.User)
	}
	// {} changes nothing
	n := db.updates
	if code, _ = put(c, `{}`); code != 200 || db.updates != n {
		t.Errorf("empty body: %d, %d saves", code, db.updates-n)
	}
}

func TestProfileFieldsValidation(t *testing.T) {
	db := newMemStore()
	c := newClient(t, quick(newService(t, db, nil)).Handler())
	if rec := c.do("PUT", "/api/me/profile", `{"country":"BY"}`, jsonHdr...); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	c.do("POST", "/api/auth/guest", "")
	before := *db.users[c.me().User.ID]

	cases := map[string]string{
		`{"country":"XX"}`:                                "страны",
		`{"country":"by"}`:                                "страны",
		`{"country":"BYY"}`:                               "страны",
		`{"country":" BY"}`:                               "страны",
		`{"country":"Беларусь"}`:                          "страны",
		`{"country":"<b>"}`:                               "страны",
		`{"firstName":"Анна1"}`:                           "Имя",
		`{"firstName":"` + strings.Repeat("я", 41) + `"}`: "Имя",
		`{"lastName":"<script>"}`:                         "Фамилия",
		`{"lastName":"` + strings.Repeat("я", 41) + `"}`:  "Фамилия",
		`{"location":"a\nb"}`:                             "Местоположение",
		`{"location":"` + strings.Repeat("я", 61) + `"}`:  "Местоположение",
		`{"about":"` + strings.Repeat("я", 501) + `"}`:    "500",
		`{"about":"a\u0000b"}`:                            "Обо мне",
		`{"avatar":"preset:nope"}`:                        "аватара",
		`{"avatar":"https://evil.test/x.png"}`:            "аватара",
		`{"avatar":"google"}`:                             "Google",
		`{"name":"я"}`:                                    "коротк",
	}
	for body, frag := range cases {
		if msg := errorOf(c, body); !strings.Contains(msg, frag) {
			t.Errorf("%s: message %q does not mention %q", body, msg, frag)
		}
	}
	// wrong types and junk
	for _, body := range []string{`nope`, `{"country":5}`, `{"about":["a"]}`, `{"firstName":{"x":1}}`, `[]`, ``} {
		if rec := c.do("PUT", "/api/me/profile", body, jsonHdr...); rec.Code != http.StatusBadRequest {
			t.Errorf("%q: %d, want 400", body, rec.Code)
		}
	}
	// one bad field refuses the whole request: the good ones are not saved either
	if msg := errorOf(c, `{"firstName":"Анна","country":"XX"}`); msg == "" {
		t.Error("no message")
	}
	if rec := c.do("PUT", "/api/me/profile", `{"country":"BY"}`, "Content-Type", "text/plain"); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form content type: %d", rec.Code)
	}
	if after := db.users[before.ID]; !reflect.DeepEqual(after, &before) || db.updates != 0 {
		t.Errorf("a refused request changed the user: %+v (%d saves)", *after, db.updates)
	}
	// a body over the limit is refused, not read to the end
	if rec := c.do("PUT", "/api/me/profile", `{"about":"`+strings.Repeat("я", 6000)+`"}`, jsonHdr...); rec.Code != http.StatusBadRequest {
		t.Errorf("huge body: %d", rec.Code)
	}
}

func TestProfileFieldsRateLimit(t *testing.T) {
	db := newMemStore()
	svc := newService(t, db, nil) // the real limiters
	c := newClient(t, svc.Handler())
	c.do("POST", "/api/auth/guest", "")

	// invalid input and "nothing changed" do not use up the allowance
	errorOf(c, `{"country":"XX"}`)
	if code, _ := put(c, `{}`); code != 200 {
		t.Fatalf("empty: %d", code)
	}
	if code, _ := put(c, `{"country":"BY"}`); code != 200 {
		t.Fatalf("first save: %d", code)
	}
	// the same value again is not a change
	if code, _ := put(c, `{"country":"BY"}`); code != 200 {
		t.Errorf("same value: %d", code)
	}
	// a second, different save right away is refused with the time to wait
	rec := c.do("PUT", "/api/me/profile", `{"country":"PL"}`, jsonHdr...)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "Подождите") {
		t.Errorf("second save: %d %s (Retry-After %q)", rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}
	if got := c.me().User.Country; got != "BY" {
		t.Errorf("a limited request changed the country to %q", got)
	}
	// once the wait is over it goes through
	svc.fields.release(c.me().User.ID)
	if code, r := put(c, `{"country":"PL"}`); code != 200 || r.User.Country != "PL" {
		t.Errorf("after the wait: %d %+v", code, r.User)
	}
	// another person has their own allowance
	c2 := newClient(t, svc.Handler())
	c2.do("POST", "/api/auth/guest", "")
	if code, _ := put(c2, `{"country":"BY"}`); code != 200 {
		t.Errorf("other user: %d", code)
	}

	// the name keeps its own, longer wait: a refused name leaves the other fields unsaved and gives their allowance back
	svc.fields.release(c.me().User.ID)
	if code, _ := put(c, `{"name":"Первое имя"}`); code != 200 {
		t.Fatalf("name: %d", code)
	}
	svc.fields.release(c.me().User.ID)
	rec = c.do("PUT", "/api/me/profile", `{"name":"Второе имя","country":"DE"}`, jsonHdr...)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "раза в минуту") {
		t.Errorf("name too soon: %d %s", rec.Code, rec.Body)
	}
	if u := c.me().User; u.Country != "PL" || u.Name != "Первое имя" {
		t.Errorf("a refused name saved the rest: %+v", u)
	}
	if code, r := put(c, `{"country":"DE"}`); code != 200 || r.User.Country != "DE" {
		t.Errorf("the fields allowance was not given back: %d", code)
	}
}

func TestProfileFieldsStoreFailureGivesAllowanceBack(t *testing.T) {
	db := newMemStore()
	svc := newService(t, db, nil)
	c := newClient(t, svc.Handler())
	c.do("POST", "/api/auth/guest", "")
	db.updateErr = errors.New("db down")
	rec := c.do("PUT", "/api/me/profile", `{"name":"Анна","country":"BY"}`, jsonHdr...)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("failing store: %d %s", rec.Code, rec.Body)
	}
	db.updateErr = nil
	if code, r := put(c, `{"name":"Анна","country":"BY"}`); code != 200 || r.User.Country != "BY" || r.User.Name != "Анна" {
		t.Errorf("retry after the failure: %d %+v", code, r.User)
	}
}

func TestAvatarChoices(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-av", Name: "Анна", Picture: "https://img.test/a.png"})
	c := newClient(t, quick(newService(t, db, g)).Handler())
	callback(c, startGoogle(t, c), "good-code")
	u := c.me().User
	if u.AvatarURL != "https://img.test/a.png" {
		t.Fatalf("a new Google user shows the photo by default: %+v", u)
	}
	if u.GoogleAvatarURL != "https://img.test/a.png" {
		t.Errorf("googleAvatarUrl: %q", u.GoogleAvatarURL)
	}

	// a ready-made avatar, then the letter, then back to the photo
	if _, r := put(c, `{"avatar":"preset:bolt"}`); r.User.AvatarURL != "preset:bolt" || r.User.GoogleAvatarURL != "https://img.test/a.png" {
		t.Errorf("preset: %+v", r.User)
	}
	if _, r := put(c, `{"avatar":""}`); r.User.AvatarURL != "" {
		t.Errorf("letter: %+v", r.User)
	}
	if _, r := put(c, `{"avatar":"google"}`); r.User.AvatarURL != "https://img.test/a.png" {
		t.Errorf("google: %+v", r.User)
	}
	// a made-up photo address is never taken from the client
	errorOf(c, `{"avatar":"https://evil.test/x.png"}`)
	if got := c.me().User.AvatarURL; got != "https://img.test/a.png" {
		t.Errorf("avatar after a refused address: %q", got)
	}
}

func TestGoogleSignInKeepsTheChosenAvatar(t *testing.T) {
	db := newMemStore()
	photo := func(url string) *Service {
		return quick(newService(t, db, fakeGoogle(t, profile{Sub: "sub-keep", Name: "Анна", Picture: url})))
	}

	// a guest picks a ready-made avatar, then links Google: the choice stays, the photo is remembered
	c := newClient(t, photo("https://img.test/1.png").Handler())
	c.do("POST", "/api/auth/guest", "")
	put(c, `{"avatar":"preset:star","country":"BY","about":"Привет"}`)
	callback(c, startGoogle(t, c), "good-code")
	u := c.me().User
	if u.Kind != store.KindGoogle || u.AvatarURL != "preset:star" || u.GoogleAvatarURL != "https://img.test/1.png" || u.Country != "BY" || u.About != "Привет" {
		t.Fatalf("after linking: %+v", u)
	}

	// signing in again, with a new photo on Google's side, does not override the choice
	c2 := newClient(t, photo("https://img.test/2.png").Handler())
	callback(c2, startGoogle(t, c2), "good-code")
	u = c2.me().User
	if u.AvatarURL != "preset:star" || u.GoogleAvatarURL != "https://img.test/2.png" {
		t.Errorf("after signing in again: %+v", u)
	}

	// the letter is a choice too
	put(c2, `{"avatar":""}`)
	c3 := newClient(t, photo("https://img.test/3.png").Handler())
	callback(c3, startGoogle(t, c3), "good-code")
	if u = c3.me().User; u.AvatarURL != "" || u.GoogleAvatarURL != "https://img.test/3.png" {
		t.Errorf("letter was overridden: %+v", u)
	}

	// someone who shows the photo gets the new one
	put(c3, `{"avatar":"google"}`)
	c4 := newClient(t, photo("https://img.test/4.png").Handler())
	callback(c4, startGoogle(t, c4), "good-code")
	if u = c4.me().User; u.AvatarURL != "https://img.test/4.png" {
		t.Errorf("the photo was not refreshed: %+v", u)
	}
}

func TestGuestWithoutChoiceGetsThePhotoOnLinking(t *testing.T) {
	db := newMemStore()
	g := fakeGoogle(t, profile{Sub: "sub-plain", Name: "Анна", Picture: "https://img.test/p.png"})
	c := newClient(t, newService(t, db, g).Handler())
	c.do("POST", "/api/auth/guest", "")
	callback(c, startGoogle(t, c), "good-code")
	if u := c.me().User; u.AvatarURL != "https://img.test/p.png" {
		t.Errorf("%+v", u)
	}
}

func TestSafePhoto(t *testing.T) {
	good := []string{"https://lh3.googleusercontent.com/a/x=s96-c", "http://x.test/a.png"}
	for _, u := range good {
		if safePhoto(u) != u {
			t.Errorf("safePhoto(%q) dropped", u)
		}
	}
	for _, u := range []string{"", "preset:crown", "javascript:alert(1)", "data:image/png;base64,AA", "//x.test/a.png", "https://x.test/" + strings.Repeat("a", 520)} {
		if got := safePhoto(u); got != "" {
			t.Errorf("safePhoto(%q) = %q", u, got)
		}
	}
}

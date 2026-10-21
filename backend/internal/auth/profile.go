package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"dyanis-chess/backend/internal/countries"
	"dyanis-chess/backend/internal/store"
)

// The display name: what a person may call themselves.
const (
	minNameLen      = 2
	maxNameLen      = 24
	nameChangeEvery = time.Minute // one change per user per this long
	maxProfileBody  = 8 << 10     // 500 characters of text take up to 2 KB in UTF-8

	// deleteConfirm is what the client must send to delete an account: a bare
	// DELETE from a script or an image tag does nothing.
	deleteConfirm = "УДАЛИТЬ"
)

// generatedGuestName is what guestName() makes; a guest who kept it has not
// chosen a name, so Google's replaces it when they sign in.
var generatedGuestName = regexp.MustCompile(`^Гость \d{4}$`)

// validName trims a name, folds runs of spaces into one and checks it. The
// message is in Russian, for the person. Letters (any alphabet), digits, space,
// hyphen, underscore and full stop are allowed; control characters, emoji and
// markup characters are not, and there must be a letter or a digit.
func validName(in string) (string, error) {
	var b strings.Builder
	space := false
	letters := 0
	for _, r := range strings.TrimSpace(in) {
		switch {
		case unicode.IsSpace(r):
			if r != ' ' && r != ' ' {
				return "", errNameChars // a tab or a line break
			}
			space = true
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			letters++
		case r == '-' || r == '_' || r == '.':
		default:
			return "", errNameChars
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	name := b.String()
	n := utf8.RuneCountInString(name)
	switch {
	case n < minNameLen:
		return "", fmt.Errorf("имя слишком короткое: нужно от %d до %d символов", minNameLen, maxNameLen)
	case n > maxNameLen:
		return "", fmt.Errorf("имя слишком длинное: нужно от %d до %d символов", minNameLen, maxNameLen)
	case letters == 0:
		return "", errors.New("в имени должна быть хотя бы одна буква или цифра")
	}
	return name, nil
}

var errNameChars = errors.New("в имени можно использовать буквы, цифры, пробел, дефис, подчёркивание и точку")

// nameLimiter lets a user change the name once per window. Kept in memory: a
// restart forgets it, which only means one extra change is allowed.
type nameLimiter struct {
	window time.Duration
	mu     sync.Mutex
	last   map[string]time.Time
}

func newNameLimiter(window time.Duration) *nameLimiter {
	return &nameLimiter{window: window, last: map[string]time.Time{}}
}

// allow reports whether the user may change the name now; if not, how long to wait.
// A change that is allowed is recorded at once (release undoes it).
func (l *nameLimiter) allow(id string) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if t, ok := l.last[id]; ok && now.Sub(t) < l.window {
		return false, l.window - now.Sub(t)
	}
	l.last[id] = now
	if len(l.last) > 10000 { // forget the idle so the map cannot grow forever
		for k, t := range l.last {
			if now.Sub(t) >= l.window {
				delete(l.last, k)
			}
		}
	}
	return true, 0
}

func (l *nameLimiter) release(id string) {
	l.mu.Lock()
	delete(l.last, id)
	l.mu.Unlock()
}

func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	// A cross-site form cannot send this content type without a preflight.
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "ожидался JSON")
		return false
	}
	return true
}

// profileRequest is the body of PUT /api/me/profile: every field is optional and only those sent
// are changed (an empty string clears a field). Avatar is "" (the letter of the name), "google" (the
// photo Google sent) or "preset:<id>".
type profileRequest struct {
	Name      *string `json:"name"`
	FirstName *string `json:"firstName"`
	LastName  *string `json:"lastName"`
	Location  *string `json:"location"`
	About     *string `json:"about"`
	Country   *string `json:"country"`
	Avatar    *string `json:"avatar"`
}

// plan validates the request against the current user and returns what actually changes (fields that
// already have the asked value are left out, so repeating a save counts as nothing). A message is
// in Russian, for the person.
func (req *profileRequest) plan(u *store.User) (up store.ProfileUpdate, err error) {
	if req.Name != nil {
		name, err := validName(*req.Name)
		if err != nil {
			return up, errors.New(capitalize(err.Error()) + ".")
		}
		if name != u.Name {
			up.Name = &name
		}
	}
	text := func(in *string, cur string, check func(string) (string, error)) (*string, error) {
		if in == nil {
			return nil, nil
		}
		v, err := check(*in)
		if err != nil || v == cur {
			return nil, err
		}
		return &v, nil
	}
	if up.FirstName, err = text(req.FirstName, u.FirstName, func(v string) (string, error) { return validPersonName(v, "Имя") }); err != nil {
		return up, err
	}
	if up.LastName, err = text(req.LastName, u.LastName, func(v string) (string, error) { return validPersonName(v, "Фамилия") }); err != nil {
		return up, err
	}
	if up.Location, err = text(req.Location, u.Location, validLocation); err != nil {
		return up, err
	}
	if up.About, err = text(req.About, u.About, validAbout); err != nil {
		return up, err
	}
	if up.Country, err = text(req.Country, u.Country, func(v string) (string, error) {
		if v != "" && !countries.Valid(v) {
			return "", errors.New("Такой страны нет в списке.")
		}
		return v, nil
	}); err != nil {
		return up, err
	}
	if up.AvatarURL, err = text(req.Avatar, u.AvatarURL, func(v string) (string, error) { return resolveAvatar(v, u.GoogleAvatarURL) }); err != nil {
		return up, err
	}
	return up, nil
}

// apply copies the changes to a user value.
func applyUpdate(u *store.User, up store.ProfileUpdate) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&u.Name, up.Name)
	set(&u.FirstName, up.FirstName)
	set(&u.LastName, up.LastName)
	set(&u.Location, up.Location)
	set(&u.About, up.About)
	set(&u.Country, up.Country)
	set(&u.AvatarURL, up.AvatarURL)
}

func (s *Service) putProfile(w http.ResponseWriter, r *http.Request) {
	u := s.UserFromRequest(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "нужно войти")
		return
	}
	if !requireJSON(w, r) {
		return
	}
	var req profileRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProfileBody))
	if err != nil || json.Unmarshal(body, &req) != nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON с полями профиля")
		return
	}
	up, err := req.plan(u)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	done := func() {
		writeJSON(w, http.StatusOK, map[string]any{"user": toJSON(u), "google": s.googleEnabled()})
	}
	otherFields := up.FirstName != nil || up.LastName != nil || up.Location != nil || up.About != nil ||
		up.Country != nil || up.AvatarURL != nil
	if up.Name == nil && !otherFields { // nothing to change, nothing to count
		done()
		return
	}
	// Both allowances are taken before anything is saved; a refusal gives back what was taken.
	if otherFields {
		if ok, wait := s.fields.allow(u.ID); !ok {
			secs := int(wait.Seconds()) + 1
			w.Header().Set("Retry-After", fmt.Sprint(secs))
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("Слишком часто. Подождите ещё %d с.", secs))
			return
		}
	}
	if up.Name != nil {
		if ok, wait := s.names.allow(u.ID); !ok {
			if otherFields {
				s.fields.release(u.ID)
			}
			secs := int(wait.Seconds()) + 1
			w.Header().Set("Retry-After", fmt.Sprint(secs))
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("Имя можно менять не чаще раза в минуту. Подождите ещё %d с.", secs))
			return
		}
	}
	if err := s.cfg.Store.UpdateProfile(r.Context(), u.ID, up); err != nil {
		if up.Name != nil {
			s.names.release(u.ID)
		}
		if otherFields {
			s.fields.release(u.ID)
		}
		log.Printf("auth: update profile: %v", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить профиль.")
		return
	}
	applyUpdate(u, up)
	done()
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// deleteMe removes the signed-in account with everything that is theirs (see
// store.DeleteUser for what goes and what stays) and ends the session. The
// body must be {"confirm":"УДАЛИТЬ"}.
func (s *Service) deleteMe(w http.ResponseWriter, r *http.Request) {
	u := s.UserFromRequest(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "нужно войти")
		return
	}
	if !requireJSON(w, r) {
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProfileBody))
	if err != nil || json.Unmarshal(body, &req) != nil || req.Confirm != deleteConfirm {
		writeError(w, http.StatusBadRequest, "Нужно подтвердить удаление.")
		return
	}
	switch err := s.cfg.Store.DeleteUser(r.Context(), u.ID); {
	case err == nil:
	case errors.Is(err, store.ErrActiveGame):
		writeError(w, http.StatusConflict, "У вас есть незаконченная партия с другим игроком. Доиграйте её или сдайтесь, и тогда аккаунт можно будет удалить.")
		return
	case errors.Is(err, store.ErrNotFound): // deleted a moment ago from another tab: the goal is reached
	default:
		log.Printf("auth: delete user: %v", err)
		writeError(w, http.StatusInternalServerError, "Не удалось удалить аккаунт. Попробуйте позже.")
		return
	}
	s.names.release(u.ID)
	// The sessions went with the user; the browser still holds the cookie.
	s.setCookie(w, r, sessionCookie, "", "/", -1)
	writeJSON(w, http.StatusOK, map[string]any{"user": nil})
}

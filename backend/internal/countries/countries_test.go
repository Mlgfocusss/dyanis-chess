package countries

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var codeRe = regexp.MustCompile(`^[A-Z]{2}$`)
var cyrillic = regexp.MustCompile(`\p{Cyrillic}`)

func TestTable(t *testing.T) {
	// 249 is the number of officially assigned ISO 3166-1 alpha-2 codes.
	if n := len(List()); n != 249 {
		t.Errorf("%d countries, want 249", n)
	}
	seen := map[string]bool{}
	seenRu := map[string]string{}
	for _, c := range List() {
		if !codeRe.MatchString(c.Code) {
			t.Errorf("code %q is not two capital letters", c.Code)
		}
		if seen[c.Code] {
			t.Errorf("duplicate code %s", c.Code)
		}
		seen[c.Code] = true
		if strings.TrimSpace(c.Ru) == "" || !cyrillic.MatchString(c.Ru) {
			t.Errorf("%s has no Russian name: %q", c.Code, c.Ru)
		}
		if strings.TrimSpace(c.En) == "" {
			t.Errorf("%s has no English name", c.Code)
		}
		if c.Ru != strings.TrimSpace(c.Ru) || c.En != strings.TrimSpace(c.En) {
			t.Errorf("%s: names have stray spaces", c.Code)
		}
		if other, dup := seenRu[c.Ru]; dup {
			t.Errorf("%s and %s share the Russian name %q", other, c.Code, c.Ru)
		}
		seenRu[c.Ru] = c.Code
	}
	for _, code := range []string{"BY", "RU", "UA", "KZ", "PL", "DE", "US", "GB", "FR", "CN", "IN", "AM", "GE"} {
		if !Valid(code) {
			t.Errorf("%s is missing", code)
		}
	}
	if NameRu("BY") != "Беларусь" || NameRu("RU") != "Россия" || NameRu("XX") != "" {
		t.Errorf("names: %q %q %q", NameRu("BY"), NameRu("RU"), NameRu("XX"))
	}
}

func TestValid(t *testing.T) {
	for _, code := range []string{"", "by", "BYY", "B", "XX", "ZZ", " BY", "BY ", "XK", "AN", "SU", "1A"} {
		if Valid(code) {
			t.Errorf("Valid(%q) = true", code)
		}
	}
	if c, ok := Lookup("BY"); !ok || c.En != "Belarus" {
		t.Errorf("Lookup(BY) = %+v %v", c, ok)
	}
}

func TestSortedByRussianName(t *testing.T) {
	names := make([]string, 0, len(List()))
	for _, c := range List() {
		names = append(names, strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(c.Ru, "ё", "е"), "Ё", "Е")))
	}
	if !sort.StringsAreSorted(names) {
		t.Error("List is not sorted by the Russian name")
	}
}

func TestHandler(t *testing.T) {
	h := Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/countries", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=86400") || !strings.Contains(cc, "public") {
		t.Errorf("Cache-Control: %q", cc)
	}
	var got struct{ Countries []Country }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Countries) != 249 {
		t.Fatalf("body: %v, %d countries", err, len(got.Countries))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/countries", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

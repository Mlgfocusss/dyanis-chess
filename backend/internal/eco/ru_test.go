package eco

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"
)

// allNames returns every distinct name in the reference data.
func allNames(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, letter := range []string{"a", "b", "c", "d", "e"} {
		data, err := dataFS.ReadFile("data/" + letter + ".tsv")
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(line, "\r")
			if i == 0 || line == "" {
				continue
			}
			f := strings.Split(line, "\t")
			if len(f) != 3 {
				t.Fatalf("%s.tsv line %d: %d fields", letter, i+1, len(f))
			}
			if !seen[f[1]] {
				seen[f[1]] = true
				out = append(out, f[1])
			}
		}
	}
	return out
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func TestEveryNameIsTranslated(t *testing.T) {
	names := allNames(t)
	if len(names) < 3000 {
		t.Fatalf("only %d names", len(names))
	}
	for _, n := range names {
		ru := TranslateName(n)
		if ru == n {
			t.Errorf("%q: not translated", n)
		}
		if !hasCyrillic(ru) {
			t.Errorf("%q: no Cyrillic in %q", n, ru)
		}
		if strings.TrimSpace(ru) != ru || strings.Contains(ru, "  ") {
			t.Errorf("%q: stray spaces in %q", n, ru)
		}
		if r := []rune(ru)[0]; !unicode.IsUpper(r) && !unicode.IsDigit(r) {
			t.Errorf("%q: %q does not start with a capital", n, ru)
		}
	}
}

// Pieces that are only move notation (no words) legitimately have no Cyrillic.
func TestNotationPiecesAreMoves(t *testing.T) {
	for en, ru := range ruPieces() {
		if hasCyrillic(ru) {
			continue
		}
		if en != ru {
			t.Errorf("piece %q → %q has no Cyrillic and differs from the original", en, ru)
		}
		for _, w := range strings.Fields(en) {
			if strings.Trim(w, "KQRBN") == w && len(w) > 3 {
				t.Errorf("piece %q looks like words, not notation", en)
			}
		}
	}
}

func TestDictionaryHasNoSpareEntries(t *testing.T) {
	used := map[string]bool{}
	for _, n := range allNames(t) {
		pieces, _ := splitName(n)
		for _, p := range pieces {
			used[p] = true
		}
	}
	for en := range ruPieces() {
		if !used[en] {
			t.Errorf("piece %q is in ru.tsv but in no opening name", en)
		}
	}
}

func TestTranslateExact(t *testing.T) {
	cases := map[string]string{
		"Italian Game: Two Knights Defense":                            "Итальянская партия: защита двух коней",
		"Sicilian Defense: Najdorf Variation":                          "Сицилианская защита: вариант Найдорфа",
		"Sicilian Defense: Najdorf Variation, Poisoned Pawn Variation": "Сицилианская защита: вариант Найдорфа, вариант отравленной пешки",
		"Queen's Gambit Declined":                                      "Отказанный ферзевый гамбит",
		"Ruy Lopez: Berlin Defense":                                    "Испанская партия (Рюи Лопес): берлинская защита",
		"Caro-Kann Defense: Advance Variation":                         "Защита Каро — Канн: продвинутый вариант",
		"King's Indian Defense":                                        "Староиндийская защита",
		"French Defense, Classical, 4. Bg5":                            "Французская защита, классический вариант, 4. Bg5",
		"Queen's Indian Defense, with e3, Bb4+ Line":                   "Ферзевоиндийская защита, с e3, линия Bb4+",
		"Vienna Gambit, with Max Lange Defense":                        "Венский гамбит, с защитой Макса Ланге",
	}
	for en, want := range cases {
		if got := TranslateName(en); got != want {
			t.Errorf("TranslateName(%q) = %q, want %q", en, got, want)
		}
	}
}

// The Russian name keeps the order and the separators of the English one.
func TestTranslateKeepsOrderAndSeparators(t *testing.T) {
	dict := map[string]string{"Alpha Opening": "дебют альфа", "Beta Variation": "вариант бета", "Gamma Line": "линия гамма"}
	got := translateWith(dict, "Alpha Opening: Beta Variation, Gamma Line")
	if want := "Дебют альфа: вариант бета, линия гамма"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := translateWith(dict, "Alpha Opening, Gamma Line"); got != "Дебют альфа, линия гамма" {
		t.Errorf("comma form: %q", got)
	}
	// One unknown piece: the whole English name comes back, never a half-translated mix.
	if got := translateWith(dict, "Alpha Opening: Unknown Variation"); got != "Alpha Opening: Unknown Variation" {
		t.Errorf("unknown piece: %q", got)
	}
}

func TestTranslateFallback(t *testing.T) {
	if got := TranslateName("Totally Made Up Opening"); got != "Totally Made Up Opening" {
		t.Errorf("unknown name: %q", got)
	}
	if got := TranslateName(""); got != "" {
		t.Errorf("empty: %q", got)
	}
}

func TestOpeningCarriesNameRU(t *testing.T) {
	o, ok := Default().Lookup(hashAfter(t, "1. e4 e5 2. Nf3 Nc6 3. Bb5 Nf6"))
	if !ok {
		t.Fatal("Berlin not found")
	}
	if o.Name != "Ruy Lopez: Berlin Defense" || o.NameRU != "Испанская партия (Рюи Лопес): берлинская защита" {
		t.Errorf("got %+v", o)
	}
	raw, _ := json.Marshal(o)
	if !strings.Contains(string(raw), `"name":"Ruy Lopez: Berlin Defense"`) || !strings.Contains(string(raw), `"nameRu":"Испанская`) {
		t.Errorf("json = %s", raw)
	}
	// A book parsed from custom text has no dictionary entry: NameRU falls back to the English name.
	b, err := Parse("eco\tname\tpgn\nA00\tShort\t1. e4\n")
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := b.Lookup(hashAfter(t, "1. e4")); o.NameRU != "Short" {
		t.Errorf("fallback NameRU = %q", o.NameRU)
	}
}

func TestDefaultBookIgnoresRussianFile(t *testing.T) {
	// ru.tsv is embedded next to the line files but must not be read as lines.
	if _, err := dataFS.ReadFile("data/ru.tsv"); err != nil {
		t.Fatal(err)
	}
	if Default().Len() < 3000 {
		t.Errorf("book has %d positions", Default().Len())
	}
}

func TestParsePieces(t *testing.T) {
	if _, err := parsePieces("segment\tru\nA\n"); err == nil {
		t.Error("one-field line accepted")
	}
	if _, err := parsePieces("segment\tru\nA\tб\nA\tв\n"); err == nil {
		t.Error("duplicate accepted")
	}
	m, err := parsePieces("segment\tru\nA\tб\n\n")
	if err != nil || m["A"] != "б" {
		t.Errorf("got %v %v", m, err)
	}
}

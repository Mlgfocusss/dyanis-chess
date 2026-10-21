package eco

import (
	"bufio"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Russian names. data/ru.tsv is a dictionary of components, not of whole
// names: every lichess opening name is a family ("Sicilian Defense"), usually
// followed by ": " and variations separated by ", " ("Najdorf Variation, Poisoned
// Pawn Variation"). Each such piece has one Russian rendering, and a name is
// translated piece by piece, keeping the original separators, so one table
// of about 2200 pieces covers all 3177 names.

var (
	ruOnce  sync.Once
	ruPiece map[string]string
)

// ruPieces returns the component dictionary, loaded from the embedded file on
// first use. A malformed file panics: it is a build defect caught by tests.
func ruPieces() map[string]string {
	ruOnce.Do(func() {
		data, err := dataFS.ReadFile("data/ru.tsv")
		if err != nil {
			panic(err)
		}
		m, err := parsePieces(string(data))
		if err != nil {
			panic(fmt.Sprintf("eco: data/ru.tsv: %v", err))
		}
		ruPiece = m
	})
	return ruPiece
}

// parsePieces reads "english<TAB>russian" lines; the first line is a header.
func parsePieces(tsv string) (map[string]string, error) {
	m := make(map[string]string, 2400)
	sc := bufio.NewScanner(strings.NewReader(tsv))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if n == 1 && strings.HasPrefix(line, "segment\t") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 2 || f[0] == "" || f[1] == "" {
			return nil, fmt.Errorf("line %d: want 2 non-empty tab-separated fields", n)
		}
		if _, dup := m[f[0]]; dup {
			return nil, fmt.Errorf("line %d: duplicate piece %q", n, f[0])
		}
		m[f[0]] = f[1]
	}
	return m, sc.Err()
}

// splitName cuts an English opening name into its pieces and the separators
// that stood between them (len(seps) == len(pieces)-1). A separator is ": "
// or ", ".
func splitName(name string) (pieces, seps []string) {
	start := 0
	for i := 0; i+1 < len(name); i++ {
		if (name[i] == ':' || name[i] == ',') && name[i+1] == ' ' {
			pieces = append(pieces, name[start:i])
			seps = append(seps, name[i:i+2])
			start = i + 2
			i++
		}
	}
	pieces = append(pieces, name[start:])
	return pieces, seps
}

// TranslateName renders an English lichess opening name in Russian, for
// example "Italian Game: Two Knights Defense" gives "Итальянская партия:
// защита двух коней". A name with a piece the dictionary does not know is
// returned unchanged (never empty), so callers can show the result as is.
func TranslateName(english string) string {
	if english == "" {
		return ""
	}
	return translateWith(ruPieces(), english)
}

func translateWith(dict map[string]string, english string) string {
	pieces, seps := splitName(english)
	var b strings.Builder
	for i, p := range pieces {
		ru, ok := dict[p]
		if !ok {
			return english
		}
		if i > 0 {
			b.WriteString(seps[i-1])
		}
		b.WriteString(ru)
	}
	return upperFirst(b.String())
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || unicode.IsUpper(r) || !unicode.IsLetter(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

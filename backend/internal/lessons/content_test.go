package lessons

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"dyanis-chess/engine"
)

func mustLoad(t *testing.T) *Library {
	t.Helper()
	lib, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestLoadContent(t *testing.T) {
	lib := mustLoad(t)
	if len(lib.Sections) != 5 {
		t.Fatalf("%d sections, want 5", len(lib.Sections))
	}
	if lib.Total() < 24 {
		t.Errorf("%d lessons, want at least 24", lib.Total())
	}
	for _, sec := range lib.Sections {
		if len(sec.Lessons) < 4 {
			t.Errorf("section %s has %d lessons", sec.ID, len(sec.Lessons))
		}
		for _, l := range sec.Lessons {
			j := lib.Lesson(l.ID)
			if j == nil || len(j.Steps) != len(l.Steps) || j.Exercises < 2 {
				t.Errorf("lesson %s is not built: %+v", l.ID, j)
			}
		}
	}
}

func TestEveryContentFileIsLoaded(t *testing.T) {
	files, err := filepath.Glob("content/*.json")
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, f := range files {
		have = append(have, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	sort.Strings(have)
	want := append([]string(nil), sectionFiles...)
	sort.Strings(want)
	if strings.Join(have, ",") != strings.Join(want, ",") {
		t.Errorf("files %v, loaded %v", have, want)
	}
}

// The icons a lesson may wear must exist in the site's Icon.jsx.
func TestIconsExistInTheSite(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "ui", "Icon.jsx"))
	if err != nil {
		t.Skipf("no web sources: %v", err)
	}
	for name := range icons {
		if !regexp.MustCompile(`(?m)^  ` + name + `:`).Match(src) {
			t.Errorf("icon %q is not in Icon.jsx", name)
		}
	}
}

// Every text is in Russian and carries no emoji (the site has none).
func TestTextsAreRussianWithoutEmoji(t *testing.T) {
	lib := mustLoad(t)
	check := func(where, s string) {
		if s == "" {
			return
		}
		cyr := false
		for _, r := range s {
			if unicode.Is(unicode.Cyrillic, r) {
				cyr = true
			}
			if r > 0xFFFF || (r >= 0x2600 && r <= 0x27BF) || r == 0xFE0F {
				t.Errorf("%s: emoji %q in %q", where, string(r), s)
			}
		}
		if !cyr {
			t.Errorf("%s: no Russian in %q", where, s)
		}
	}
	for _, sec := range lib.Sections {
		check("section "+sec.ID, sec.Title)
		check("section "+sec.ID, sec.Description)
		for _, l := range sec.Lessons {
			check("lesson "+l.ID, l.Title)
			check("lesson "+l.ID, l.Description)
			for i, st := range l.Steps {
				w := l.ID + " step " + string(rune('1'+i))
				check(w, st.Text)
				check(w, st.Title)
				check(w, st.Hint)
				check(w, st.Success)
				check(w, st.Miss)
				for _, m := range st.Mistakes {
					check(w, m.Message)
				}
			}
		}
	}
}

func TestBadContentIsRefused(t *testing.T) {
	ok := Step{Type: TypeText, FEN: engine.StartFEN, Text: "Текст."}
	ex := Step{Type: TypeExercise, FEN: engine.StartFEN, Text: "Ход.", Hint: "п", Success: "у", Solutions: [][]string{{"e2e4"}}}
	lesson := func(steps ...Step) Lesson {
		return Lesson{ID: "t", Title: "Т", Description: "Д", Icon: "lessons", Minutes: 5, Steps: steps}
	}
	good := lesson(ok, ok, ok, ex, ex)
	sec := func(ls ...Lesson) []Section {
		return []Section{{ID: "s", Title: "С", Description: "О", Lessons: ls}}
	}
	if _, err := newLibrary(sec(good)); err != nil {
		t.Fatalf("a good lesson is refused: %v", err)
	}
	mut := func(f func(l *Lesson)) []Section {
		l := lesson(append([]Step(nil), good.Steps...)...)
		f(&l)
		return sec(l)
	}
	bad := map[string][]Section{
		"bad fen":           mut(func(l *Lesson) { l.Steps[0].FEN = "nonsense" }),
		"illegal solution":  mut(func(l *Lesson) { l.Steps[3].Solutions = [][]string{{"e2e5"}} }),
		"illegal reply":     mut(func(l *Lesson) { l.Steps[3].Solutions = [][]string{{"e2e4", "e7e4"}} }),
		"no solutions":      mut(func(l *Lesson) { l.Steps[3].Solutions = nil }),
		"illegal demo":      mut(func(l *Lesson) { l.Steps[0].Demo = []string{"e2e4", "e2e4"} }),
		"unknown type":      mut(func(l *Lesson) { l.Steps[0].Type = "video" }),
		"no hint":           mut(func(l *Lesson) { l.Steps[3].Hint = "" }),
		"too few steps":     mut(func(l *Lesson) { l.Steps = l.Steps[:4] }),
		"too few exercises": mut(func(l *Lesson) { l.Steps[4] = ok }),
		"bad arrow":         mut(func(l *Lesson) { l.Steps[0].Arrows = [][]string{{"e2", "z9"}} }),
		"bad colour":        mut(func(l *Lesson) { l.Steps[0].Arrows = [][]string{{"e2", "e4", "pink"}} }),
		"bad highlight":     mut(func(l *Lesson) { l.Steps[0].Highlights = []string{"i1"} }),
		"unknown icon":      mut(func(l *Lesson) { l.Icon = "rocket" }),
		"empty text":        mut(func(l *Lesson) { l.Steps[0].Text = " " }),
		"a text with a solution": mut(func(l *Lesson) {
			l.Steps[0].Solutions = [][]string{{"e2e4"}}
		}),
		"mistake that is a solution": mut(func(l *Lesson) {
			l.Steps[3].Mistakes = []Mistake{{Move: "e2e4", Message: "Нет."}}
		}),
		"mistake that is illegal": mut(func(l *Lesson) {
			l.Steps[3].Mistakes = []Mistake{{Move: "e2e5", Message: "Нет."}}
		}),
		"two replies to one move": mut(func(l *Lesson) {
			l.Steps[3].Solutions = [][]string{{"e2e4", "e7e5", "g1f3"}, {"e2e4", "e7e6", "g1f3"}}
		}),
		"a solution stops where another goes on": mut(func(l *Lesson) {
			l.Steps[3].Solutions = [][]string{{"e2e4"}, {"e2e4", "e7e5", "g1f3"}}
		}),
		"repeated lesson id": func() []Section { return sec(good, good) }(),
	}
	for name, secs := range bad {
		if _, err := newLibrary(secs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTreeAcceptsEveryMateAndKeepsScriptedReplies(t *testing.T) {
	// A king behind its pawns on the 8th rank: Ra8 and Rb8 both mate, only one is written down.
	pos, _ := engine.ParseFEN("6k1/5ppp/8/8/8/8/R7/1R4K1 w - - 0 1")
	root, err := buildTree(pos, [][]string{{"a2a8"}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasAccept(root, "b1b8") || !hasAccept(root, "a2a8") {
		t.Errorf("both mates must be accepted: %+v", root.Moves)
	}
	if len(root.Legal) < 10 {
		t.Errorf("legal moves: %v", root.Legal)
	}
	// a chain with a reply and a second move
	start := engine.NewPosition()
	root, err = buildTree(start, [][]string{{"e2e4", "e7e5", "g1f3"}, {"d2d4"}})
	if err != nil {
		t.Fatal(err)
	}
	a := root.Moves[0]
	if a.UCI != "e2e4" || a.Reply == nil || a.Reply.UCI != "e7e5" || a.Next == nil || a.Next.FEN != a.Reply.FEN {
		t.Fatalf("tree: %+v", a)
	}
	if len(a.Next.Moves) != 1 || a.Next.Moves[0].UCI != "g1f3" || a.Next.Moves[0].Reply != nil {
		t.Errorf("second move: %+v", a.Next.Moves)
	}
	if len(a.Next.Legal) != 29 {
		t.Errorf("legal moves after 1.e4 e5: %d", len(a.Next.Legal))
	}
}

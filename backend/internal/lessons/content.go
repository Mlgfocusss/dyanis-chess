// Package lessons serves the lessons ("Уроки"): short courses of explanation steps and exercises
// that the site shows move by move.
//
// The lessons are written by hand and live in content/*.json, one file per section, embedded in
// the binary. There is no neural network and no outside service: the text and the positions are
// ours, and the engine's rules check them. Load reads and validates everything (every FEN parses,
// every move of every demonstration and solution is legal, the ids are unique) and the server
// refuses to start on a content error, so a broken lesson never reaches a person.
//
// The browser has no rules of chess, so for an exercise the server prepares a tree of the moves
// that solve it (see Node): the legal moves of every position the person moves in, the positions
// after each accepted move and after the opponent's scripted reply, as the puzzles package does
// for a puzzle.
package lessons

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"dyanis-chess/engine"
)

//go:embed content/*.json
var contentFS embed.FS

// sectionFiles are the files of content/ in the order of the sections on the page.
var sectionFiles = []string{"basics", "tactics", "openings", "endgames", "strategy"}

// Step types.
const (
	TypeText     = "text"
	TypeExercise = "exercise"
)

// Colours of arrows and highlighted squares: names of the site's colour tokens (--primary and the
// rest in web/src/index.css), so that a light theme recolours them.
var colours = map[string]bool{"primary": true, "info": true, "warn": true, "danger": true}

// Icons a lesson may wear: names in web/src/ui/Icon.jsx (a test compares).
var icons = map[string]bool{
	"lessons": true, "star": true, "bolt": true, "puzzles": true, "openings": true, "crown": true, "flag": true,
	"trophy": true, "flame": true, "board": true, "bot": true, "duel": true, "coach": true, "check": true, "clock": true,
	"play": true, "list": true,
}

var (
	squareRe = regexp.MustCompile(`^[a-h][1-8]$`)
	uciRe    = regexp.MustCompile(`^[a-h][1-8][a-h][1-8][qrbn]?$`)
	idRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,38}$`)
)

// Section is a group of lessons ("Основы", "Тактика").
type Section struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Lessons     []Lesson `json:"lessons"`
}

// Lesson is a sequence of steps.
type Lesson struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Minutes     int    `json:"minutes"`
	Steps       []Step `json:"steps"`
}

// Step is one screen of a lesson: an explanation or an exercise.
type Step struct {
	Type        string     `json:"type"`                  // "text" or "exercise"
	Title       string     `json:"title,omitempty"`       // a short heading
	FEN         string     `json:"fen"`                   // the position on the board
	Orientation string     `json:"orientation,omitempty"` // "white" or "black"; by default the side to move for an exercise, White for text
	Text        string     `json:"text"`                  // the explanation, or the task of an exercise
	Arrows      [][]string `json:"arrows,omitempty"`      // [from, to] or [from, to, colour]
	Highlights  []string   `json:"highlights,omitempty"`  // "e4" or "e4:colour"
	Demo        []string   `json:"demo,omitempty"`        // a text step: moves (UCI) the "Показать" button plays from the position

	// An exercise only. The person moves first (the side to move in FEN). Every solution is a chain
	// of moves, the person's and the opponent's by turns: [mine, reply, mine, ...]. A chain may end on
	// either side's move. Where the person moves, any move that gives checkmate is accepted as well.
	Solutions [][]string `json:"solutions,omitempty"`
	Hint      string     `json:"hint,omitempty"`    // shown by "Подсказка"
	Success   string     `json:"success,omitempty"` // the praise, with the reason the move is good
	Miss      string     `json:"miss,omitempty"`    // for a wrong move that has no Mistakes entry
	Mistakes  []Mistake  `json:"mistakes,omitempty"`
	Claim     string     `json:"claim,omitempty"` // what the tests verify with the engine: "mate1", "mate2", "best", "wins-material", ...
}

// Mistake is a typical wrong move and what to tell about it. Move is a move (UCI) or a square
// ("e2": any move of the piece standing there). At, when set, limits it to the person's n-th move
// of the exercise (0 is the first).
type Mistake struct {
	Move    string `json:"move"`
	Message string `json:"message"`
	At      *int   `json:"at,omitempty"`
}

// Library is the validated content with the lessons ready for the browser.
type Library struct {
	Sections []Section
	order    []string // ids of the lessons in the order of the course
	byID     map[string]*built
}

type built struct {
	section *Section
	lesson  *Lesson
	index   int // position in order
	json    *LessonJSON
}

// Load reads, validates and prepares the embedded content.
func Load() (*Library, error) {
	var secs []Section
	for _, name := range sectionFiles {
		data, err := contentFS.ReadFile("content/" + name + ".json")
		if err != nil {
			return nil, fmt.Errorf("lessons: %w", err)
		}
		var s Section
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields() // a misspelt field would silently lose a hint
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("lessons: %s.json: %w", name, err)
		}
		if s.ID != name {
			return nil, fmt.Errorf("lessons: %s.json has the section id %q", name, s.ID)
		}
		secs = append(secs, s)
	}
	return newLibrary(secs)
}

// newLibrary validates sections and prepares their lessons.
func newLibrary(secs []Section) (*Library, error) {
	lib := &Library{Sections: secs, byID: map[string]*built{}}
	var errs []error
	secSeen := map[string]bool{}
	for si := range lib.Sections {
		sec := &lib.Sections[si]
		if !idRe.MatchString(sec.ID) || secSeen[sec.ID] {
			errs = append(errs, fmt.Errorf("section id %q is bad or repeated", sec.ID))
		}
		secSeen[sec.ID] = true
		if sec.Title == "" || sec.Description == "" || len(sec.Lessons) == 0 {
			errs = append(errs, fmt.Errorf("section %s: title, description and lessons are required", sec.ID))
		}
		for li := range sec.Lessons {
			l := &sec.Lessons[li]
			if prev, dup := lib.byID[l.ID]; dup || !idRe.MatchString(l.ID) {
				errs = append(errs, fmt.Errorf("lesson id %q is bad or repeated (section %s, first in %v)", l.ID, sec.ID, prev != nil))
				continue
			}
			lj, err := buildLesson(l)
			if err != nil {
				errs = append(errs, fmt.Errorf("lesson %s: %w", l.ID, err))
				continue
			}
			lib.byID[l.ID] = &built{section: sec, lesson: l, index: len(lib.order), json: lj}
			lib.order = append(lib.order, l.ID)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("lessons: bad content:\n%w", err)
	}
	return lib, nil
}

// Lesson returns a lesson ready for the browser, or nil.
func (l *Library) Lesson(id string) *LessonJSON {
	b := l.byID[id]
	if b == nil {
		return nil
	}
	out := *b.json
	out.Section = SectionRef{ID: b.section.ID, Title: b.section.Title}
	if b.index+1 < len(l.order) {
		n := l.byID[l.order[b.index+1]]
		out.Next = &LessonRef{ID: n.lesson.ID, Title: n.lesson.Title}
	}
	return &out
}

// Total is the number of lessons.
func (l *Library) Total() int { return len(l.order) }

// StepCount is the number of steps of a lesson, 0 for an unknown one.
func (l *Library) StepCount(id string) int {
	if b := l.byID[id]; b != nil {
		return len(b.lesson.Steps)
	}
	return 0
}

// ----------------------------------------------------------------- validation

// Limits that keep the lessons the size of a lesson: a few short steps.
const (
	minSteps     = 5
	maxSteps     = 9
	minExercises = 2
	maxTextRunes = 520
)

func buildLesson(l *Lesson) (*LessonJSON, error) {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	if l.Title == "" || l.Description == "" {
		add("title and description are required")
	}
	if !icons[l.Icon] {
		add("unknown icon %q", l.Icon)
	}
	if l.Minutes < 1 || l.Minutes > 30 {
		add("minutes %d out of range", l.Minutes)
	}
	if n := len(l.Steps); n < minSteps || n > maxSteps {
		add("%d steps, want %d to %d", n, minSteps, maxSteps)
	}
	out := &LessonJSON{ID: l.ID, Title: l.Title, Description: l.Description, Icon: l.Icon, Minutes: l.Minutes}
	exercises := 0
	for i := range l.Steps {
		sj, err := buildStep(&l.Steps[i])
		if err != nil {
			add("step %d: %v", i+1, err)
			continue
		}
		if sj.Type == TypeExercise {
			exercises++
		}
		out.Steps = append(out.Steps, *sj)
	}
	if exercises < minExercises {
		add("%d exercises, want at least %d", exercises, minExercises)
	}
	out.Exercises = exercises
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func checkArrows(arrows [][]string) ([][]string, error) {
	var out [][]string
	for _, a := range arrows {
		if len(a) != 2 && len(a) != 3 {
			return nil, fmt.Errorf("arrow %v must be [from, to] or [from, to, colour]", a)
		}
		if !squareRe.MatchString(a[0]) || !squareRe.MatchString(a[1]) || a[0] == a[1] {
			return nil, fmt.Errorf("arrow %v has a bad square", a)
		}
		colour := "info"
		if len(a) == 3 {
			colour = a[2]
		}
		if !colours[colour] {
			return nil, fmt.Errorf("arrow %v has an unknown colour", a)
		}
		out = append(out, []string{a[0], a[1], colour})
	}
	return out, nil
}

func checkHighlights(hs []string) error {
	for _, h := range hs {
		sq, colour, has := strings.Cut(h, ":")
		if !squareRe.MatchString(sq) || (has && !colours[colour]) {
			return fmt.Errorf("highlight %q is bad", h)
		}
	}
	return nil
}

// PassMove in a demonstration hands the move to the other side without a move on the board, so
// that a piece can be shown going from square to square with nothing in its way.
const PassMove = "--"

// passTurn is the position with the other side to move. The author keeps the kings out of check:
// a pass in a position where the side that passes gives check would leave an impossible position.
func passTurn(p *engine.Position) (*engine.Position, error) {
	f := strings.Fields(p.FEN())
	if len(f) < 4 {
		return nil, errors.New("short fen")
	}
	if f[1] == "w" {
		f[1] = "b"
	} else {
		f[1] = "w"
	}
	f[3] = "-" // no en passant after a pass
	q, err := engine.ParseFEN(strings.Join(f, " "))
	if err != nil {
		return nil, err
	}
	return q, nil
}

func findMove(pos *engine.Position, uci string) (engine.Move, bool) {
	for _, m := range pos.LegalMoves() {
		if m.String() == uci {
			return m, true
		}
	}
	return engine.Move{}, false
}

func buildStep(st *Step) (*StepJSON, error) {
	if st.Type != TypeText && st.Type != TypeExercise {
		return nil, fmt.Errorf("unknown type %q", st.Type)
	}
	pos, err := engine.ParseFEN(st.FEN)
	if err != nil {
		return nil, fmt.Errorf("fen: %w", err)
	}
	if strings.TrimSpace(st.Text) == "" {
		return nil, errors.New("text is required")
	}
	if n := utf8.RuneCountInString(st.Text); n > maxTextRunes {
		return nil, fmt.Errorf("text has %d characters, want at most %d", n, maxTextRunes)
	}
	arrows, err := checkArrows(st.Arrows)
	if err != nil {
		return nil, err
	}
	if err := checkHighlights(st.Highlights); err != nil {
		return nil, err
	}
	orient := st.Orientation
	switch orient {
	case "white", "black":
	case "":
		orient = "white"
		if st.Type == TypeExercise && pos.SideToMove() == engine.Black {
			orient = "black"
		}
	default:
		return nil, fmt.Errorf("orientation %q", st.Orientation)
	}
	out := &StepJSON{Type: st.Type, Title: st.Title, FEN: pos.FEN(), Orientation: orient, Text: st.Text, Arrows: arrows, Highlights: st.Highlights}

	if st.Type == TypeText {
		if len(st.Solutions) > 0 || st.Hint != "" || st.Success != "" || len(st.Mistakes) > 0 || st.Miss != "" {
			return nil, errors.New("a text step has no solutions, hint, success or mistakes")
		}
		p := pos.Copy()
		for i, u := range st.Demo {
			if u == PassMove {
				q, err := passTurn(p)
				if err != nil {
					return nil, fmt.Errorf("demo move %d (pass): %w", i+1, err)
				}
				p = q
				out.Demo = append(out.Demo, DemoMove{UCI: PassMove, FEN: p.FEN()})
				continue
			}
			m, ok := findMove(p, u)
			if !ok {
				return nil, fmt.Errorf("demo move %d (%s) is not legal in %s", i+1, u, p.FEN())
			}
			san := p.SAN(m)
			p.Play(m)
			out.Demo = append(out.Demo, DemoMove{UCI: u, SAN: san, FEN: p.FEN()})
		}
		return out, nil
	}

	if len(st.Demo) > 0 {
		return nil, errors.New("an exercise has no demo")
	}
	if st.Hint == "" || st.Success == "" {
		return nil, errors.New("an exercise needs a hint and a success text")
	}
	root, err := buildTree(pos, st.Solutions)
	if err != nil {
		return nil, err
	}
	out.Hint, out.Success, out.Miss, out.Tree = st.Hint, st.Success, st.Miss, root
	for _, mk := range st.Mistakes {
		if mk.Message == "" || !(squareRe.MatchString(mk.Move) || uciRe.MatchString(mk.Move)) {
			return nil, fmt.Errorf("mistake %q is bad", mk.Move)
		}
		if err := checkMistake(root, mk); err != nil {
			return nil, err
		}
		out.Mistakes = append(out.Mistakes, mk)
	}
	return out, nil
}

// checkMistake refuses a "mistake" that is a solution, or a move no position of the exercise allows.
func checkMistake(root *Node, mk Mistake) error {
	var nodes []*Node
	for depth, level := range nodesByDepth(root) {
		if mk.At == nil || *mk.At == depth {
			nodes = append(nodes, level...)
		}
	}
	found := false
	for _, n := range nodes {
		for _, l := range n.Legal {
			if l != mk.Move && !(len(mk.Move) == 2 && l[:2] == mk.Move) {
				continue
			}
			found = true
			// a square names any move of the piece, and the right ones are judged first
			if len(mk.Move) > 2 && hasAccept(n, l) {
				return fmt.Errorf("mistake %s is also a solution", mk.Move)
			}
		}
	}
	if !found {
		return fmt.Errorf("mistake %s is not a legal move of the exercise", mk.Move)
	}
	return nil
}

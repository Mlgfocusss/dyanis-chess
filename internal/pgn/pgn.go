// Package pgn reads and writes chess games in Portable Game Notation.
//
// Parse understands what real-world PGN files (lichess, chess.com,
// ChessBase exports) contain: the seven-tag roster plus any extra tags,
// SAN movetext with "{...}" and ";..." comments, NAG codes ($1, !?),
// nested "(...)" variations (skipped — only the main line is kept),
// "%" escape lines, the "[%clk h:mm:ss]" clock annotations both sites
// embed in comments, a custom start position via the FEN/SetUp tags,
// and several games per file. Every move is resolved against the real
// legal-move list (movegen.ParseSAN), so a Game that comes out of
// Parse is guaranteed playable move by move.
package pgn

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/movegen"
)

// Move is one half-move of the main line.
type Move struct {
	SAN  string     // normalized SAN as produced by movegen.SAN ("Nf3", "exd5", "O-O", "e8=Q#")
	Move board.Move // the resolved engine move
	// Clock is the time left on the mover's clock after the move, from
	// a "[%clk ...]" annotation; negative if the file had none.
	Clock   time.Duration
	Comment string // the move's text comment with %-annotations removed
}

// Game is one parsed game.
type Game struct {
	Tags     map[string]string
	TagOrder []string // tag names in file order, for faithful re-writing
	// StartFEN is the position the game starts from: the FEN tag when
	// present (with SetUp "1"), else the standard initial position.
	StartFEN string
	Moves    []Move
	// Result is the movetext termination marker ("1-0", "0-1",
	// "1/2-1/2" or "*"); falls back to the Result tag.
	Result string
}

// Tag returns the value of tag name, or "" if absent.
func (g *Game) Tag(name string) string { return g.Tags[name] }

// Board returns a fresh board at the game's start position.
func (g *Game) Board() (*board.Board, error) {
	if g.StartFEN == "" || g.StartFEN == board.StartFEN {
		return board.NewInitialBoard(), nil
	}
	return board.FromFEN(g.StartFEN)
}

var (
	tagRe = regexp.MustCompile(`^\[\s*([A-Za-z0-9_]+)\s+"((?:[^"\\]|\\.)*)"\s*\]\s*$`)
	clkRe = regexp.MustCompile(`\[%clk\s+(\d+):(\d+):(\d+(?:\.\d+)?)\]`)
	annRe = regexp.MustCompile(`\[%[^\]]*\]`)
)

// ParseError reports where a game could not be read.
type ParseError struct {
	Game int // 1-based index of the game within the input
	Ply  int // 1-based half-move that failed, 0 if the failure is not about a move
	Msg  string
}

func (e *ParseError) Error() string {
	if e.Ply > 0 {
		return fmt.Sprintf("pgn: game %d, ply %d: %s", e.Game, e.Ply, e.Msg)
	}
	return fmt.Sprintf("pgn: game %d: %s", e.Game, e.Msg)
}

// ParseString parses all games in s.
func ParseString(s string) ([]*Game, error) { return Parse(strings.NewReader(s)) }

// ParseOne parses s and returns its first game. It is an error if s
// contains none.
func ParseOne(s string) (*Game, error) {
	games, err := ParseString(s)
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, fmt.Errorf("pgn: no game found")
	}
	return games[0], nil
}

// Parse reads every game from r. On the first malformed game it
// returns the games read before it together with a *ParseError.
func Parse(r io.Reader) ([]*Game, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)

	var games []*Game
	var tags []string // raw tag lines of the game being collected
	var text strings.Builder
	inMoves := false

	flush := func() error {
		if len(tags) == 0 && strings.TrimSpace(text.String()) == "" {
			return nil
		}
		g, err := buildGame(len(games)+1, tags, text.String())
		tags, inMoves = nil, false
		text.Reset()
		if err != nil {
			return err
		}
		games = append(games, g)
		return nil
	}

	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			line = strings.TrimPrefix(line, "\xef\xbb\xbf") // UTF-8 BOM
			first = false
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "%") { // escape mechanism: line is ignored
			continue
		}
		if tagRe.MatchString(trimmed) {
			if inMoves { // a tag after movetext starts the next game
				if err := flush(); err != nil {
					return games, err
				}
			}
			tags = append(tags, trimmed)
			continue
		}
		if trimmed == "" {
			if inMoves && endsGame(text.String()) {
				if err := flush(); err != nil {
					return games, err
				}
			}
			continue
		}
		inMoves = true
		text.WriteString(line)
		text.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return games, err
	}
	if err := flush(); err != nil {
		return games, err
	}
	return games, nil
}

// endsGame reports whether movetext already ends in a termination
// marker, so a blank line after it cleanly closes the game even when
// the next game has no tags at all.
func endsGame(movetext string) bool {
	f := strings.Fields(stripComments(movetext))
	if len(f) == 0 {
		return false
	}
	switch f[len(f)-1] {
	case "1-0", "0-1", "1/2-1/2", "*":
		return true
	}
	return false
}

// stripComments removes {..}, ;.. and nested (..) text, for cheap
// token inspection only (buildGame does its own, comment-preserving scan).
func stripComments(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '{':
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				return b.String()
			}
			i += j
		case c == ';':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

func buildGame(idx int, tagLines []string, movetext string) (*Game, error) {
	g := &Game{Tags: map[string]string{}}
	for _, l := range tagLines {
		m := tagRe.FindStringSubmatch(l)
		name, val := m[1], strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(m[2])
		if _, dup := g.Tags[name]; !dup {
			g.TagOrder = append(g.TagOrder, name)
		}
		g.Tags[name] = val
	}

	g.StartFEN = board.StartFEN
	if fen := g.Tags["FEN"]; fen != "" && g.Tags["SetUp"] != "0" {
		g.StartFEN = fen
	}
	b, err := g.Board()
	if err != nil {
		return nil, &ParseError{Game: idx, Msg: fmt.Sprintf("bad FEN tag: %v", err)}
	}

	toks, err := tokenize(movetext)
	if err != nil {
		return nil, &ParseError{Game: idx, Msg: err.Error()}
	}

	for _, t := range toks {
		switch t.kind {
		case tokResult:
			g.Result = t.text
		case tokComment:
			if n := len(g.Moves); n > 0 {
				mv := &g.Moves[n-1]
				if c := clkRe.FindStringSubmatch(t.text); c != nil {
					h, _ := strconv.Atoi(c[1])
					m, _ := strconv.Atoi(c[2])
					s, _ := strconv.ParseFloat(c[3], 64)
					mv.Clock = time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
						time.Duration(s*float64(time.Second))
				}
				if txt := strings.TrimSpace(annRe.ReplaceAllString(t.text, "")); txt != "" {
					if mv.Comment != "" {
						mv.Comment += " "
					}
					mv.Comment += txt
				}
			}
		case tokMove:
			ply := len(g.Moves) + 1
			m, err := movegen.ParseSAN(b, t.text)
			if err != nil {
				return nil, &ParseError{Game: idx, Ply: ply, Msg: err.Error()}
			}
			san := movegen.SAN(b, m)
			b.MakeMove(m)
			g.Moves = append(g.Moves, Move{SAN: san, Move: m, Clock: -1})
		}
	}
	if g.Result == "" {
		g.Result = g.Tags["Result"]
	}
	if g.Result == "" {
		g.Result = "*"
	}
	return g, nil
}

type tokKind int

const (
	tokMove tokKind = iota
	tokComment
	tokResult
)

type token struct {
	kind tokKind
	text string
}

// tokenize splits movetext into SAN moves, comments and the result
// marker, discarding move numbers, NAGs and variations.
func tokenize(s string) ([]token, error) {
	var out []token
	depth := 0 // variation nesting
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '{':
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				return nil, fmt.Errorf("unterminated { comment")
			}
			if depth == 0 {
				out = append(out, token{tokComment, s[i+1 : i+j]})
			}
			i += j + 1
		case c == ';':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			if depth == 0 {
				out = append(out, token{tokComment, s[i+1 : i+j]})
			}
			i += j
		case c == '(':
			depth++
			i++
		case c == ')':
			if depth == 0 {
				return nil, fmt.Errorf("unbalanced )")
			}
			depth--
			i++
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t\r\n{};()", rune(s[j])) {
				j++
			}
			word := s[i:j]
			i = j
			if depth > 0 {
				continue
			}
			switch word {
			case "1-0", "0-1", "1/2-1/2", "*":
				out = append(out, token{tokResult, word})
				continue
			}
			if word[0] == '$' { // NAG
				continue
			}
			// "12." / "12..." / "12.e4" (number glued to the move)
			if word[0] >= '0' && word[0] <= '9' {
				k := 0
				for k < len(word) && word[k] >= '0' && word[k] <= '9' {
					k++
				}
				if k < len(word) && word[k] == '.' {
					for k < len(word) && word[k] == '.' {
						k++
					}
					word = word[k:]
					if word == "" {
						continue
					}
				}
			}
			if word == "--" || word == "Z0" { // null move: not representable
				return nil, fmt.Errorf("null moves are not supported")
			}
			out = append(out, token{tokMove, word})
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced (")
	}
	return out, nil
}

// String renders the game as PGN: tags in their original order (the
// seven-tag roster first for tags this Game has), then SAN movetext
// wrapped at 80 columns. Clock annotations are written back when known.
func (g *Game) String() string {
	var sb strings.Builder
	written := map[string]bool{}
	roster := []string{"Event", "Site", "Date", "Round", "White", "Black", "Result"}
	put := func(name, val string) {
		fmt.Fprintf(&sb, "[%s \"%s\"]\n", name, strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(val))
		written[name] = true
	}
	for _, n := range roster {
		if v, ok := g.Tags[n]; ok {
			put(n, v)
		} else if n == "Result" {
			put(n, g.Result)
		} else {
			put(n, "?")
		}
	}
	for _, n := range g.TagOrder {
		if !written[n] {
			put(n, g.Tags[n])
		}
	}
	if g.StartFEN != "" && g.StartFEN != board.StartFEN && !written["FEN"] {
		put("SetUp", "1")
		put("FEN", g.StartFEN)
	}
	sb.WriteByte('\n')

	var words []string
	startPly := 0 // ply parity of the first move: 0 = white to move
	if g.StartFEN != "" && g.StartFEN != board.StartFEN {
		if f := strings.Fields(g.StartFEN); len(f) > 1 && f[1] == "b" {
			startPly = 1
		}
	}
	num := 1
	if g.StartFEN != "" && g.StartFEN != board.StartFEN {
		if f := strings.Fields(g.StartFEN); len(f) > 5 {
			if n, err := strconv.Atoi(f[5]); err == nil && n > 0 {
				num = n
			}
		}
	}
	for i, m := range g.Moves {
		blackMove := (i+startPly)%2 == 1
		if !blackMove {
			words = append(words, fmt.Sprintf("%d.", num+(i+startPly)/2))
		} else if i == 0 {
			words = append(words, fmt.Sprintf("%d...", num))
		}
		words = append(words, m.SAN)
		if m.Clock >= 0 || m.Comment != "" {
			var c string
			if m.Clock >= 0 {
				secs := m.Clock.Seconds()
				c = fmt.Sprintf("[%%clk %d:%02d:%s]", int(secs)/3600, int(secs)/60%60, trimSec(secs-float64(int(secs)/60*60)))
			}
			if m.Comment != "" {
				if c != "" {
					c += " "
				}
				c += m.Comment
			}
			words = append(words, "{"+c+"}")
		}
	}
	words = append(words, g.Result)

	line := 0
	for _, w := range words {
		if line > 0 && line+1+len(w) > 80 {
			sb.WriteByte('\n')
			line = 0
		}
		if line > 0 {
			sb.WriteByte(' ')
			line++
		}
		sb.WriteString(w)
		line += len(w)
	}
	sb.WriteByte('\n')
	return sb.String()
}

func trimSec(s float64) string {
	str := strconv.FormatFloat(s, 'f', 1, 64)
	str = strings.TrimSuffix(str, ".0")
	if f, _ := strconv.ParseFloat(str, 64); f < 10 {
		str = "0" + str
	}
	return str
}

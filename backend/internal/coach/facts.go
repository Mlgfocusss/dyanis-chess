package coach

import (
	"fmt"
	"sort"
	"strings"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/engine"
)

// maxLine is how many half-moves of an engine line the coach is told about.
const maxLine = 8

// Facts is everything the coach knows about one move of a reviewed game: what
// was played, how the engine judged it, and the engine's own lines. The coach
// does not calculate chess; it puts these facts into words, so every move it
// may name is in them.
type Facts struct {
	MoveNumber int    // of the full move, as in the game's notation
	Mover      string // "white" or "black"
	Played     string // the move, SAN
	Class      analysis.Class
	Loss       float64 // win-probability points lost by the mover
	Forced     bool    // the only legal move

	// The mover's chances before and after the move, in percent, and the
	// same in the words of an evaluation ("+1.2", "-0.4", "мат в 3").
	WinBefore, WinAfter   float64
	EvalBefore, EvalAfter string

	// BestSAN is the engine's choice when it differs from the move played.
	BestSAN string
	// BestLine is the engine's line from the position before the move; ReplyLine
	// is the engine's line from the position after the move played, that is,
	// what the opponent does next.
	BestLine, ReplyLine []string
	// Material (pawns, the mover's view): before the move, at the end of
	// BestLine, and at the end of Played followed by ReplyLine.
	MaterialBefore, MaterialBest, MaterialReply int
	// Terminal says the move ended the game ("checkmate", "stalemate", ...).
	Terminal string

	Opening string // the named opening reached, if any
	Book    bool   // the move is opening theory

	FENBefore string
	Position  string // the position before the move, piece by piece
}

// BuildFacts collects the facts of move ply (1-based) of a report.
func BuildFacts(rep *analysis.Report, ply int) (Facts, error) {
	if rep == nil || ply < 1 || ply > len(rep.Moves) || len(rep.Evals) <= ply {
		return Facts{}, fmt.Errorf("coach: no move %d in the report", ply)
	}
	mv := rep.Moves[ply-1]
	before, after := rep.Evals[ply-1], rep.Evals[ply]
	white := mv.Color == "white"

	f := Facts{
		MoveNumber: (ply + 1) / 2,
		Mover:      mv.Color,
		Played:     mv.SAN,
		Class:      mv.Class,
		Loss:       mv.Loss,
		Forced:     mv.Forced,
		WinBefore:  pov(before.WinPct, white),
		WinAfter:   pov(after.WinPct, white),
		FENBefore:  before.FEN,
		Position:   Describe(before.FEN),
		Book:       mv.Book,
		Terminal:   after.Terminal,
	}
	f.EvalBefore = evalText(before, white)
	f.EvalAfter = evalText(after, white)
	if mv.Opening != nil {
		f.Opening = mv.Opening.Name
		if mv.Opening.NameRU != "" {
			f.Opening = mv.Opening.NameRU // the coach writes in Russian, so it is told the Russian name
		}
	}
	if mv.BestSAN != "" && mv.BestSAN != mv.SAN {
		f.BestSAN = mv.BestSAN
	}
	f.BestLine = clip(before.PV)
	f.ReplyLine = clip(after.PV)

	f.MaterialBefore = povInt(material(before.FEN), white)
	f.MaterialBest = povInt(material(playLine(before.FEN, f.BestLine)), white)
	f.MaterialReply = povInt(material(playLine(after.FEN, f.ReplyLine)), white)
	return f, nil
}

func clip(line []string) []string {
	if len(line) > maxLine {
		line = line[:maxLine]
	}
	return append([]string(nil), line...)
}

func pov(whitePct float64, white bool) float64 {
	if white {
		return whitePct
	}
	return 100 - whitePct
}

func povInt(whiteView int, white bool) int {
	if white {
		return whiteView
	}
	return -whiteView
}

// evalText writes an evaluation from the mover's point of view: "+1.2" is good
// for the mover. The engine's centipawns are shown in pawns.
func evalText(ev analysis.PositionEval, white bool) string {
	if ev.Terminal != "" {
		if ev.Terminal == "checkmate" {
			if (ev.Score > 0) == white {
				return "мат поставлен"
			}
			return "поставлен мат"
		}
		return "ничья"
	}
	if ev.Mate != 0 {
		m := povInt(ev.Mate, white)
		if m > 0 {
			return fmt.Sprintf("мат в %d", m)
		}
		return fmt.Sprintf("мат против, через %d", -m)
	}
	cp := povInt(ev.Score, white)
	return fmt.Sprintf("%+.1f", float64(cp)/100)
}

// playLine plays a SAN line from a position and returns the last FEN reached;
// a move that does not fit ends the line there.
func playLine(fen string, sans []string) string {
	pos, err := engine.ParseFEN(fen)
	if err != nil {
		return fen
	}
	for _, s := range sans {
		m, err := pos.ParseSAN(s)
		if err != nil {
			break
		}
		pos.Play(m)
	}
	return pos.FEN()
}

var pieceValue = map[byte]int{'p': 1, 'n': 3, 'b': 3, 'r': 5, 'q': 9}

// material is White's material minus Black's, in pawns, from a FEN.
func material(fen string) int {
	board, _, _ := strings.Cut(fen, " ")
	total := 0
	for i := 0; i < len(board); i++ {
		c := board[i]
		lower := c | 0x20
		v, ok := pieceValue[lower]
		if !ok {
			continue
		}
		if c == lower {
			total -= v
		} else {
			total += v
		}
	}
	return total
}

// Describe lists the pieces of a position by side, as "Белые: Kg1 Qd1 ...;
// пешки a2 b2 ...". Piece letters are those of the notation of moves.
func Describe(fen string) string {
	board, rest, _ := strings.Cut(fen, " ")
	type placed struct {
		letter byte
		sq     string
	}
	var white, black []placed
	for r, row := range strings.Split(board, "/") {
		file := 0
		for i := 0; i < len(row); i++ {
			c := row[i]
			if c >= '1' && c <= '8' {
				file += int(c - '0')
				continue
			}
			p := placed{letter: c &^ 0x20, sq: fmt.Sprintf("%c%d", 'a'+file, 8-r)}
			if c == p.letter {
				white = append(white, p)
			} else {
				black = append(black, p)
			}
			file++
		}
	}
	order := map[byte]int{'K': 0, 'Q': 1, 'R': 2, 'B': 3, 'N': 4, 'P': 5}
	side := func(ps []placed) string {
		sort.SliceStable(ps, func(i, j int) bool {
			if order[ps[i].letter] != order[ps[j].letter] {
				return order[ps[i].letter] < order[ps[j].letter]
			}
			return ps[i].sq < ps[j].sq
		})
		var pieces, pawns []string
		for _, p := range ps {
			if p.letter == 'P' {
				pawns = append(pawns, p.sq)
			} else {
				pieces = append(pieces, string(p.letter)+p.sq)
			}
		}
		out := strings.Join(pieces, " ")
		if len(pawns) > 0 {
			out += "; пешки " + strings.Join(pawns, " ")
		}
		return out
	}
	turn := "белых"
	if strings.HasPrefix(rest, "b") {
		turn = "чёрных"
	}
	return fmt.Sprintf("Белые: %s.\nЧёрные: %s.\nХод %s.", side(white), side(black), turn)
}

// Numbered writes a line of SAN moves with move numbers, "12. Nxe5 Qxe5 13. Re1+";
// a line that begins with Black's move starts "12...Qxe5".
func Numbered(moveNumber int, whiteFirst bool, sans []string) string {
	var b strings.Builder
	n, white := moveNumber, whiteFirst
	for i, s := range sans {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch {
		case white:
			fmt.Fprintf(&b, "%d. %s", n, s)
		case i == 0:
			fmt.Fprintf(&b, "%d...%s", n, s)
		default:
			b.WriteString(s)
		}
		if !white {
			n++
		}
		white = !white
	}
	return b.String()
}

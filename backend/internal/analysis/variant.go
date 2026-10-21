package analysis

import (
	"errors"
	"fmt"
	"strings"

	"dyanis-chess/backend/internal/variants"
	"dyanis-chess/engine"
)

// Review of a game of Chess960.
//
// The engine knows only the classical castling, and a Chess960 game cannot be
// replayed by it. The rules of the game are the business of the variants
// package, so the engine is not asked about the moves at all: variants plays
// the game and gives the position before every move; each position goes to the
// engine as a FEN with the castling rights removed (the pieces may stand on any
// squares, the search and the network do not care) and the move played is judged,
// as always, by the position it led to. A castling is written by variants as the
// king taking its own rook ("e1h1") and as "O-O" or "O-O-O".
//
// What this costs: while a castling is still possible the engine does not know
// it, so it never recommends one and the "best move" shown can never be a
// castling; a castling that was played is judged by the position after it, as
// any move. A position repeated only once the rights were gone is counted by the
// engine as repeated from the first time; in a real game this is very rare.
// There is no opening book (no names for Chess960 setups), so no move is theory.

// VariantChess960 is the id of the variant this file knows.
const VariantChess960 = "chess960"

// ErrVariantUnsupported is the answer for a variant that has no review.
var ErrVariantUnsupported = errors.New("разбор партии есть только у классических шахмат и Chess960")

// SupportsVariant says whether games of the variant can be reviewed.
func SupportsVariant(id string) bool { return id == VariantChess960 }

// VariantGame is a game of a variant prepared for review: the position before
// every move (castling rights removed) and the moves.
type VariantGame struct {
	Variant string
	// FENs has one entry more than Moves: the position before each move and the last one.
	FENs  []string
	Moves []VariantMove
}

// VariantMove is one half-move as variants recorded it.
type VariantMove struct {
	SAN    string
	UCI    string // the action: "e2e4", a castling "e1h1" (the king takes its rook)
	// Shown is the move as the report writes it: the same, but a castling is the king's own
	// move from where it stood to where it stands after ("e1g1"; "g1g1" when it did not move),
	// so that the page marks the king's squares and puts the badge on the king.
	Shown string
	Castle bool
	Legal  int // how many moves the mover had
}

// NewVariantGame replays the actions of a game of the variant under the options.
func NewVariantGame(id string, o variants.Options, actions []string) (*VariantGame, error) {
	if !SupportsVariant(id) {
		return nil, ErrVariantUnsupported
	}
	v, ok := variants.Get(id)
	if !ok {
		return nil, fmt.Errorf("такого вида шахмат нет: %q", id)
	}
	st, err := v.New(o)
	if err != nil {
		return nil, err
	}
	g, ok := st.(*variants.Game)
	if !ok {
		return nil, ErrVariantUnsupported
	}
	vg := &VariantGame{Variant: id}
	vg.FENs = append(vg.FENs, noCastling(g.Position().FEN()))
	for i, a := range actions {
		pos := g.Position()
		mv := VariantMove{UCI: a, Shown: a, Legal: len(g.Moves())}
		if len(a) >= 4 {
			from, ok1 := variants.ParseSq(a[0:2])
			to, ok2 := variants.ParseSq(a[2:4])
			if ok1 && ok2 {
				pc, target := pos.B[from], pos.B[to]
				mv.Castle = pc.Kind == variants.King && target.Kind == variants.Rook && target.Side == pc.Side
				if mv.Castle {
					file := "g"
					if to.File() < from.File() {
						file = "c"
					}
					mv.Shown = a[0:2] + file + a[1:2]
				}
			}
		}
		note, err := g.Apply(a)
		if err != nil {
			return nil, fmt.Errorf("ход %d (%s): %w", i+1, a, err)
		}
		mv.SAN = note
		vg.Moves = append(vg.Moves, mv)
		vg.FENs = append(vg.FENs, noCastling(g.Position().FEN()))
	}
	return vg, nil
}

// noCastling blanks the castling field of a FEN.
func noCastling(fen string) string {
	f := strings.Fields(fen)
	if len(f) >= 3 {
		f[2] = "-"
	}
	return strings.Join(f, " ")
}

// AnalyzeVariant reviews a game of a variant (see the comment above). The
// report has Variant set and no opening.
func AnalyzeVariant(vg *VariantGame, opt Options) (*Report, error) {
	n := len(vg.Moves)
	tl := &timeline{
		boards: make([]*engine.Position, n+1),
		spare:  make([]*engine.Position, n+1),
		hashes: make([]uint64, n+1),
		moves:  make([]playedMove, n),
	}
	if len(vg.FENs) != n+1 {
		return nil, errors.New("analysis: the positions do not match the moves")
	}
	for i, fen := range vg.FENs {
		b, err := engine.ParseFEN(fen)
		if err != nil {
			return nil, fmt.Errorf("позиция %d (%s): %w", i, fen, err)
		}
		tl.boards[i], tl.spare[i], tl.hashes[i] = b.Copy(), b.Copy(), b.Hash()
	}
	tl.startBlack = tl.boards[0].SideToMove() == engine.Black
	for i, m := range vg.Moves {
		pm := playedMove{SAN: m.SAN, UCI: m.UCI, ClockMs: -1, Legal: m.Legal}
		if m.Shown != "" {
			pm.UCI = m.Shown
		}
		if !m.Castle {
			// the engine's own move: found by its text among the moves of the
			// position (which has no castling, and so no move to be missed)
			for _, lm := range tl.boards[i].LegalMoves() {
				if lm.String() == m.UCI {
					pm.Move, pm.HasMove = lm, true
					break
				}
			}
			if !pm.HasMove {
				return nil, fmt.Errorf("ход %d (%s) не найден среди ходов позиции", i+1, m.UCI)
			}
		}
		tl.moves[i] = pm
	}
	rep := review(tl, opt)
	rep.Variant = vg.Variant
	return rep, nil
}

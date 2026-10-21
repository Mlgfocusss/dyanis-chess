package variants

import (
	"errors"
	"strings"
)

// Duck chess. A turn is two actions of one side: a move of a piece, then the
// duck put on any empty square except the one it stood on. The duck is a wall:
// nothing can be taken on its square, and sliders and pawns cannot pass it.
//
// Rules chosen (the original ones):
//   - There is no check. A king may stand under attack or walk into it, and the
//     game ends when a king is taken. The game does not stop at the duck step
//     of the move that took it: the capture ends it at once.
//   - The duck is not on the board at the start. White puts it down after its
//     first move, on any empty square. After that every duck step moves it to
//     another empty square (never the one it stands on).
//   - A side with no move for a piece when its turn starts wins (the original
//     rule): the other side has locked it in with the duck.
//   - Castling follows the rules without check: the king and the rook have not
//     moved and every square between them and their destinations is empty (the
//     duck blocks too); attacked squares do not matter.
//   - The fifty-move rule and the threefold repetition still end the game in a
//     draw (the duck counts in the position). Dead material is not a draw: a
//     bare king can still take the other.
//   - A flag loses outright (a king can always be taken).
//
// Actions: a piece move as everywhere ("e2e4", "e7e8q", "e1g1"), then "@e5" to
// put the duck on e5. Turn() stays with the mover during the duck step, so the
// server keeps one clock running through both actions.

type duckRules struct {
	ClassicRules
	mid *bool // true between the piece move and the duck: the position is not a final one yet
}

func (duckRules) Check(*Position, Side) bool          { return false }
func (duckRules) CastleSafe(*Position, Side, Sq) bool { return true }
func (duckRules) Timeout(g *Game, loser Side) string  { return ResultFor(loser.Opp()) }

// Result: the side whose king is gone has lost; draws by the fifty-move rule and
// by repetition are judged only on a position that has its duck.
func (r duckRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		if p.KingSq(s) == NoSq {
			return ResultFor(s.Opp()), ReasonKingCaptured, true
		}
	}
	if r.mid != nil && *r.mid {
		return "", "", false
	}
	if p.Half >= 100 {
		return ResultDraw, ReasonFiftyMoves, true
	}
	if g.Repetitions() >= 3 {
		return ResultDraw, ReasonRepetition, true
	}
	return "", "", false
}

// duckState is a game of duck chess: a Game and the step of the turn.
type duckState struct {
	*Game
	mid bool // the piece has moved and the duck is still to be put down
}

// duckExtra is View.Extra of the duck variant.
type duckExtra struct {
	Step string `json:"step"` // "move" or "duck": what the side to act must do
	Duck string `json:"duck"` // the square of the duck, "" before the first duck step
}

func newDuck(o Options) (State, error) {
	d := &duckState{}
	g, err := NewGame(duckRules{mid: &d.mid}, o)
	if err != nil {
		return nil, err
	}
	d.Game = g
	return d, nil
}

// duckSq is where the duck stands, NoSq before it is first put down.
func (d *duckState) duckSq() Sq {
	for i, pc := range d.pos.B {
		if pc.Kind == Duck {
			return Sq(i)
		}
	}
	return NoSq
}

// Turn: during the duck step the mover still acts (the Game has already passed
// the turn to the other side).
func (d *duckState) Turn() Side {
	if d.mid {
		return d.pos.Turn.Opp()
	}
	return d.pos.Turn
}

// Legal: the piece moves, or the squares the duck may go to.
func (d *duckState) Legal() []string {
	if _, _, over := d.Game.Over(); over {
		return nil
	}
	if !d.mid {
		return d.Game.Legal()
	}
	here := d.duckSq()
	out := make([]string, 0, 64)
	for i, pc := range d.pos.B {
		if pc.Empty() && Sq(i) != here {
			out = append(out, "@"+Sq(i).String())
		}
	}
	return out
}

var errDuckSquare = errors.New("утку нельзя поставить на это поле")

// Apply plays a piece move, or puts the duck down.
func (d *duckState) Apply(action string) (string, error) {
	if _, _, over := d.Game.Over(); over {
		return "", ErrOver
	}
	if !d.mid {
		d.mid = true // the Game judges the half-finished position as such
		note, err := d.Game.Apply(action)
		if _, _, over := d.Game.Over(); err != nil || over {
			d.mid = false
		}
		if err != nil {
			return "", err
		}
		return note, nil
	}
	action = strings.TrimSpace(action)
	if len(action) != 3 || action[0] != '@' {
		return "", ErrIllegal
	}
	to, ok := ParseSq(action[1:])
	if !ok {
		return "", ErrIllegal
	}
	if !d.pos.B[to].Empty() || to == d.duckSq() {
		return "", errDuckSquare
	}
	from := d.duckSq()
	d.mid = false
	d.Game.Edit(func(p *Position) {
		if from != NoSq {
			p.B[from] = Piece{}
		}
		p.B[to] = Piece{Kind: Duck}
	})
	if _, _, over := d.Game.Over(); !over && len(d.Game.Moves()) == 0 {
		// the side to move is locked in by the duck: it wins
		d.Game.Finish(ResultFor(d.pos.Turn), ReasonNoMoves)
	}
	return action, nil
}

// View: the Game's view with the real actor, its actions and the duck's data.
func (d *duckState) View(viewer Side) View {
	v := d.Game.View(viewer)
	v.Turn = d.Turn()
	v.Legal = nil
	if viewer != NoSide && viewer == v.Turn {
		v.Legal = d.Legal()
	}
	step := "move"
	if d.mid {
		step = "duck"
	}
	duck := ""
	if sq := d.duckSq(); sq != NoSq {
		duck = sq.String()
	}
	v.Extra = duckExtra{Step: step, Duck: duck}
	return v
}

type duckVariant struct{}

func (duckVariant) Info() Info {
	return Info{
		ID:    "duck",
		Name:  "Утиные шахматы",
		Short: "После каждого хода фигурой вы ставите утку на любое свободное поле: она никого не пускает и сама не берётся.",
	}
}

func (duckVariant) New(o Options) (State, error) { return newDuck(o) }

func init() { Register(duckVariant{}) }

package variants

import (
	"math/rand"
	"strings"
)

// Dice chess. A turn is up to three moves of one side. At the start of the turn
// three dice are thrown at once and each names a kind of piece: 1 pawn, 2 knight,
// 3 bishop, 4 rook, 5 queen, 6 king (the six faces are equally likely). The three
// kinds are shown together; the side moves with them in any order it likes, each
// die once. The same piece may move more than once in a turn if the dice allow.
//
// Rules chosen (the author's answers and the assumptions around them):
//   - All three dice are thrown by the server when the turn begins, from the
//     seed of the game (Options.Rand, the order of throws is fixed), and are in
//     every snapshot from then on; the seed itself never leaves the server. A game
//     is rebuilt exactly by replaying its actions: the dice do not change after a
//     restart of the server. (Showing them only after the player presses the die
//     is the page's business: the throw is already made, there is nothing to hide
//     that the player could use.)
//   - A move is a piece move ("e2e4", "e7e8q", "e1g1") with a piece of a kind that
//     still has an unused die. The server decides which die was used: the first
//     unused one of that kind (two dice of one kind are the same die).
//   - A die whose kind has no move is skipped for good, at once: when the dice are
//     thrown, and again after every move for each die still left. A skip is never
//     taken back, even if a later move would give the kind a move. When no die is
//     left the turn ends. A turn in which none of the three kinds can move at the
//     start is lost completely and the other side goes on (new dice at once, and so
//     on until somebody can play).
//   - There is no check. A king may stand under attack or walk into it, and the
//     game ends when a king is taken (this is the "mate" of the game, even in the
//     middle of a turn). Castling is a king move: it needs the die of the king,
//     the king and rook unmoved and the squares between them empty; attacked
//     squares do not matter.
//   - A side that has no legal move with any piece when its turn begins draws the
//     game by stalemate (so that a locked position cannot loop forever).
//   - The fifty-move rule counts moves: 150 moves (50 full turns) without a
//     capture or a pawn move is a draw. There is no repetition rule: the dice
//     are not part of a position. Dead material is not a draw: a king can take a king.
//   - A flag loses outright.
//
// Actions: the piece moves ("e2e4", "e7e8q", "e1g1"). A skip is not an action:
// the server does it by itself and the notation of the action it follows tells
// of it.

type diceRules struct{ ClassicRules }

func (diceRules) Check(*Position, Side) bool          { return false }
func (diceRules) CastleSafe(*Position, Side, Sq) bool { return true }
func (diceRules) Timeout(g *Game, loser Side) string  { return ResultFor(loser.Opp()) }

// Result: the side whose king is gone has lost; 150 moves with no pawn move and
// no capture draw (three moves make a turn).
func (diceRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		if p.KingSq(s) == NoSq {
			return ResultFor(s.Opp()), ReasonKingCaptured, true
		}
	}
	if p.Half >= 150 {
		return ResultDraw, ReasonFiftyMoves, true
	}
	return "", "", false
}

// diceMoves is the number of dice thrown for a turn.
const diceMoves = 3

// diceFace is a die of the turn in progress.
type diceFace struct {
	Die string `json:"die"` // "P" "N" "B" "R" "Q" "K"
	// State: "free" (not used, a piece of the kind can move), "used" (a move was made with it), "skip" (no piece
	// of the kind could move: skipped for good).
	State string `json:"state"`
}

// diceExtra is View.Extra of the dice variant.
type diceExtra struct {
	Of   int        `json:"of"`   // 3
	Dice []diceFace `json:"dice"` // the three dice of this turn, in the order thrown
	Left int        `json:"left"` // how many dice are not used yet
	// Turns is how many times the turn has passed.
	Turns int `json:"turns"`
	// Lost tells of a whole turn lost before this one began (its three dice named kinds that could not move).
	Lost *diceLost `json:"lost,omitempty"`
}

// diceLost is a turn in which nothing could be played: whose it was and what its dice were.
type diceLost struct {
	Side Side     `json:"side"`
	Dice []string `json:"dice"`
}

const (
	diceFree uint8 = iota
	diceUsed
	diceSkip
)

// diceState is a game of dice chess: a Game, the dice and the step of the turn.
type diceState struct {
	*Game
	rng    *rand.Rand
	mover  Side // who acts; the Game's own turn is kept equal to it
	dice   [diceMoves]Kind
	st     [diceMoves]uint8 // per die: diceFree, diceUsed, diceSkip
	lead   []string         // notes of turns lost before the first move of the game, for that move's notation
	passes int              // how many times the turn has passed
	lost   *diceLost
}

func newDice(o Options) (State, error) { return newDiceWith(diceRules{}, o) }

// newDiceWith starts a game under the rules (tests use other starting positions).
func newDiceWith(r Rules, o Options) (State, error) {
	g, err := NewGame(r, o)
	if err != nil {
		return nil, err
	}
	d := &diceState{Game: g, rng: o.Rand(), mover: g.pos.Turn}
	d.lead = d.deal() // a turn lost before the first move is told with it
	return d, nil
}

var diceNames = map[Kind]string{Pawn: "пешка", Knight: "конь", Bishop: "слон", Rook: "ладья", Queen: "ферзь", King: "король"}
var sideWords = map[Side]string{White: "Белые", Black: "Чёрные"}

// Turn is the side that acts.
func (d *diceState) Turn() Side { return d.mover }

// TurnsPassed is how many times the turn has changed hands (see TurnCounter).
func (d *diceState) TurnsPassed() int { return d.passes }

// unusedKind says whether a die of the kind can still be played.
func (d *diceState) unusedKind(k Kind) bool {
	for i, dk := range d.dice {
		if dk == k && d.st[i] == diceFree {
			return true
		}
	}
	return false
}

// moves are the legal moves of the kinds that have an unused die.
func (d *diceState) moves() []Move {
	var out []Move
	for _, m := range d.Game.Moves() {
		if m.Drop == NoKind && d.unusedKind(d.pos.B[m.From].Kind) {
			out = append(out, m)
		}
	}
	return out
}

// Legal lists the moves of the kinds still to be played, to the side that acts.
func (d *diceState) Legal() []string {
	if _, _, over := d.Game.Over(); over {
		return nil
	}
	ms := d.moves()
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = Encode(d.rules, m)
	}
	return out
}

// setTurn gives the Game's turn to the side (the Game passes it after every move).
func (d *diceState) setTurn(s Side) {
	if d.pos.Turn != s {
		d.Game.Edit(func(p *Position) { p.Turn = s })
	}
}

// deal throws the three dice for the side that acts now and, while none of them
// names a kind that can move, loses that turn and gives the dice to the other
// side. It returns the notes of the lost turns.
func (d *diceState) deal() (trail []string) {
	for guard := 0; guard < 100000; guard++ {
		if _, _, over := d.Game.Over(); over {
			return trail
		}
		if len(d.Game.Moves()) == 0 {
			d.Game.Finish(ResultDraw, ReasonStalemate)
			return trail
		}
		for i := range d.dice {
			d.dice[i] = Kind(1 + d.rng.Intn(6))
			d.st[i] = diceFree
		}
		d.skipDead() // the dice that cannot be played are skipped at once
		if d.left() > 0 {
			return trail
		}
		// none of the three can move: the turn is lost
		d.lost = &diceLost{Side: d.mover}
		var names []string
		for _, k := range d.dice {
			d.lost.Dice = append(d.lost.Dice, string(rune(k.Letter())))
			names = append(names, diceNames[k])
		}
		trail = append(trail, sideWords[d.mover]+": пропуск хода ("+strings.Join(names, ", ")+")")
		d.nextSide()
	}
	return trail
}

// nextSide hands the turn to the other side.
func (d *diceState) nextSide() {
	d.mover = d.mover.Opp()
	d.passes++
	d.setTurn(d.mover)
}

// skipDead skips every free die whose kind has no move and returns the names of the kinds skipped.
func (d *diceState) skipDead() (names []string) {
	for i, k := range d.dice {
		if d.st[i] == diceFree && !d.canMove(k) {
			d.st[i] = diceSkip
			names = append(names, diceNames[k])
		}
	}
	return names
}

// left is how many dice can still be played.
func (d *diceState) left() int {
	n := 0
	for _, s := range d.st {
		if s == diceFree {
			n++
		}
	}
	return n
}

// Apply plays a move of a kind that has an unused die and uses the first such die.
// The turn goes on while a die is left that can be played, and ends otherwise.
func (d *diceState) Apply(action string) (string, error) {
	if _, _, over := d.Game.Over(); over {
		return "", ErrOver
	}
	mv, ok := Find(d.rules, d.moves(), action)
	if !ok {
		return "", ErrIllegal
	}
	actor := d.mover
	kind := d.pos.B[mv.From].Kind
	d.lost = nil
	parts := append([]string{}, d.lead...)
	d.lead = nil
	parts = append(parts, string(rune(kind.Letter()))+": "+Encode(d.rules, mv))
	d.Game.Play(mv)
	for i, k := range d.dice {
		if k == kind && d.st[i] == diceFree {
			d.st[i] = diceUsed
			break
		}
	}
	if _, _, over := d.Game.Over(); over {
		return strings.Join(parts, " | "), nil
	}
	d.setTurn(actor)                               // the Game passed the turn; the rest of the turn is still the actor's
	if skipped := d.skipDead(); len(skipped) > 0 { // what cannot move now is skipped for good
		parts = append(parts, "пропуск ("+strings.Join(skipped, ", ")+")")
	}
	if d.left() > 0 {
		return strings.Join(parts, " | "), nil
	}
	d.nextSide()
	parts = append(parts, d.deal()...)
	return strings.Join(parts, " | "), nil
}

// View: the Game's view with the dice.
func (d *diceState) View(viewer Side) View {
	v := d.Game.View(viewer)
	v.Turn = d.mover
	v.Legal = nil
	if viewer != NoSide && viewer == d.mover {
		v.Legal = d.Legal()
	}
	ex := diceExtra{Of: diceMoves, Turns: d.passes, Lost: d.lost, Left: d.left()}
	for i, k := range d.dice {
		f := diceFace{Die: string(rune(k.Letter()))}
		f.State = [...]string{"free", "used", "skip"}[d.st[i]]
		ex.Dice = append(ex.Dice, f)
	}
	v.Extra = ex
	return v
}

// canMove says whether a piece of the kind has a legal move now.
func (d *diceState) canMove(k Kind) bool {
	for _, m := range d.Game.Moves() {
		if m.Drop == NoKind && d.pos.B[m.From].Kind == k {
			return true
		}
	}
	return false
}

type diceVariant struct{}

func (diceVariant) Info() Info {
	return Info{
		ID:    "dice",
		Name:  "Dice chess",
		Short: "В начале очереди бросаются три кубика и сразу показывают три типа фигур; ходите ими в любом порядке.",
	}
}

func (diceVariant) New(o Options) (State, error) { return newDice(o) }

func init() { Register(diceVariant{}) }

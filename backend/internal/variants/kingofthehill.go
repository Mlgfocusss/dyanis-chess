package variants

// King of the Hill, as on Lichess: ordinary chess, and a king that reaches one
// of the four central squares (d4, e4, d5, e5) wins at once. Checkmate still
// wins; stalemate and the draws are classical. The king may not step onto the
// hill when the square is attacked (it is an ordinary move into check).

// ReasonKingOfTheHill is the end of a game by a king on the central squares.
const ReasonKingOfTheHill = "king-of-the-hill"

type kothRules struct{ ClassicRules }

// onHill says whether the square is d4, e4, d5 or e5.
func onHill(s Sq) bool {
	return s != NoSq && s.File() >= 3 && s.File() <= 4 && s.Rank() >= 3 && s.Rank() <= 4
}

// Result: the king on the hill first (it wins even when the move would have
// been mate for the other side: the game is over before anyone can mate), then
// the classical judgement.
func (kothRules) Result(g *Game) (string, string, bool) {
	p := g.Position()
	for _, s := range [2]Side{White, Black} {
		if onHill(p.KingSq(s)) {
			return ResultFor(s), ReasonKingOfTheHill, true
		}
	}
	return ClassicResult(g)
}

type kingOfTheHill struct{}

func (kingOfTheHill) Info() Info {
	return Info{
		ID:    "kingofthehill",
		Name:  "Царь горы",
		Short: "Обычные шахматы, но побеждает и тот, чей король первым дошёл до центра: d4, e4, d5 или e5.",
	}
}

func (kingOfTheHill) New(o Options) (State, error) { return NewGame(kothRules{}, o) }

func init() { Register(kingOfTheHill{}) }

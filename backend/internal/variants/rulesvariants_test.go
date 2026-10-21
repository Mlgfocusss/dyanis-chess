package variants

import (
	"fmt"
	"math/rand"
	"testing"
)

// Random games of atomic, King of the Hill and Horde: the state never panics,
// every legal action applies, the end is consistent with the board, and the
// same actions replayed give the same board.
func TestAtomicHillHordeRandomGames(t *testing.T) {
	games := 120
	if testing.Short() {
		games = 25
	}
	for _, id := range []string{"atomic", "kingofthehill", "horde"} {
		rng := rand.New(rand.NewSource(11))
		ended := map[string]int{}
		for n := 0; n < games; n++ {
			st, err := Replay(id, Options{Seed: int64(n)}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var actions []string
			for ply := 0; ply < 160; ply++ {
				if _, _, over := st.Over(); over {
					break
				}
				legal := st.Legal()
				if len(legal) == 0 {
					t.Fatalf("%s game %d: no legal action in a game that is not over", id, n)
				}
				a := legal[rng.Intn(len(legal))]
				if _, err := st.Apply(a); err != nil {
					t.Fatalf("%s game %d: %s: %v", id, n, a, err)
				}
				actions = append(actions, a)
				for _, side := range []Side{White, Black, NoSide} {
					v := st.View(side)
					if side == NoSide && len(v.Legal) != 0 {
						t.Fatalf("%s: the spectator was shown moves", id)
					}
				}
			}
			res, reason, over := st.Over()
			if over {
				ended[reason]++
				p := st.(interface{ Position() *Position }).Position()
				switch reason {
				case ReasonKingExploded:
					if p.KingSq(White) != NoSq && p.KingSq(Black) != NoSq || p.KingSq(White) == NoSq && p.KingSq(Black) == NoSq {
						t.Fatalf("%s: king exploded but %s", id, p.FEN())
					}
				case ReasonKingOfTheHill:
					if !onHill(p.KingSq(White)) && !onHill(p.KingSq(Black)) {
						t.Fatalf("%s: hill win without a king on the hill: %s", id, p.FEN())
					}
				case ReasonHordeDestroyed:
					if hordePieces(p) != 0 || res != ResultBlack {
						t.Fatalf("%s: horde destroyed but %s", id, p.FEN())
					}
				}
			}
			again, err := Replay(id, Options{Seed: int64(n)}, actions)
			if err != nil {
				t.Fatal(err)
			}
			a, b := st.(interface{ Position() *Position }).Position().FEN(), again.(interface{ Position() *Position }).Position().FEN()
			if a != b {
				t.Fatalf("%s game %d: replay differs: %s / %s", id, n, a, b)
			}
		}
		t.Log(id, fmt.Sprint(ended))
	}
}

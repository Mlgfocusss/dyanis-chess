package variants

import (
	"fmt"
	"strings"
)

// Chess960 (Fischer random chess): the pieces of the first rank start in one of
// 960 orders, chosen by a number 0..959 (Scharnagl's numbering: 518 is the
// classical setup). The server draws the seed; the number is the seed modulo
// 960 unless the lobby named one. The castling is the Fischer one, which the
// base already knows: the king and the rook end on the g and f (or c and d)
// files wherever they started, and a castling is written as the king taking its
// own rook ("e1h1"), because the king's two squares are not always its move.

const (
	// Chess960Count is the number of setups.
	Chess960Count = 960
	// Chess960Classic is the number of the ordinary setup.
	Chess960Classic = 518
)

// Chess960Rank is the first rank of setup n (0..959) for White, upper case, the
// a-file first: 518 gives "RNBQKBNR", 0 gives "BBQNNRKR".
func Chess960Rank(n int) (string, error) {
	if n < 0 || n >= Chess960Count {
		return "", fmt.Errorf("номер расстановки: от 0 до %d", Chess960Count-1)
	}
	var rank [8]byte
	n2, b1 := n/4, n%4
	rank[1+2*b1] = 'B' // the bishop of the light squares: b, d, f or h
	n3, b2 := n2/4, n2%4
	rank[2*b2] = 'B' // the bishop of the dark squares: a, c, e or g
	n4, q := n3/6, n3%6
	empty := func() []int {
		var out []int
		for i, c := range rank {
			if c == 0 {
				out = append(out, i)
			}
		}
		return out
	}
	rank[empty()[q]] = 'Q'
	// where the two knights stand among the five squares left
	knights := [10][2]int{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 2}, {1, 3}, {1, 4}, {2, 3}, {2, 4}, {3, 4}}
	free := empty()
	rank[free[knights[n4][0]]] = 'N'
	rank[free[knights[n4][1]]] = 'N'
	free = empty()
	rank[free[0]], rank[free[1]], rank[free[2]] = 'R', 'K', 'R'
	return string(rank[:]), nil
}

// Chess960FEN is the starting position of setup n; the castling rights name
// the files of the rooks (Shredder-FEN).
func Chess960FEN(n int) (string, error) {
	white, err := Chess960Rank(n)
	if err != nil {
		return "", err
	}
	black := strings.ToLower(white)
	var files []byte
	for i := 0; i < 8; i++ {
		if white[i] == 'R' {
			files = append(files, byte('A'+i))
		}
	}
	// the right of the rook on the h side first, as KQkq does
	rights := string([]byte{files[1], files[0]})
	return fmt.Sprintf("%s/pppppppp/8/8/8/8/PPPPPPPP/%s w %s%s - 0 1", black, white, rights, strings.ToLower(rights)), nil
}

// chess960Number is the setup of a game: the number named in the lobby, else
// the seed's. It never fails because CheckOptions has already refused a bad number.
func chess960Number(o Options) int {
	if v, ok := o.Params["position"]; ok {
		if n, ok := paramInt(v); ok && n >= 0 && n < Chess960Count {
			return n
		}
	}
	n := int(o.Seed % Chess960Count)
	if n < 0 {
		n += Chess960Count
	}
	return n
}

// paramInt reads a whole number out of a JSON-ish value.
func paramInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	}
	return 0, false
}

type fischerRandomRules struct{ ClassicRules }

func (fischerRandomRules) CastleAsKingTakesRook() bool { return true }

func (fischerRandomRules) Start(o Options) (*Position, error) {
	fen, err := Chess960FEN(chess960Number(o))
	if err != nil {
		return nil, err
	}
	return ParseFEN(fen)
}

// Extra tells the browser the number and the first rank of the setup.
func (fischerRandomRules) Extra(g *Game, _ Side) any {
	n := chess960Number(g.Options())
	rank, _ := Chess960Rank(n)
	return map[string]any{"number": n, "rank": rank}
}

type chess960 struct{}

func (chess960) Info() Info {
	return Info{
		ID:    "chess960",
		Name:  "Chess960",
		Short: "Фигуры первой горизонтали стоят в случайном порядке (одна из 960 расстановок), рокировка по правилам Фишера.",
	}
}

func (chess960) New(o Options) (State, error) { return NewGame(fischerRandomRules{}, o) }

// CheckOptions: the only choice is "position", the number 0..959 (absent: the
// server's draw). The number the game uses is written to Params as "number", so
// the record of the game has it; a rematch asks again and draws a new one unless
// the lobby fixed "position".
func (chess960) CheckOptions(o *Options) error {
	params := map[string]any{}
	_, fixed := o.Params["position"]
	for k, v := range o.Params {
		switch k {
		case "position":
			if v == nil {
				fixed = false
				continue
			}
			n, ok := paramInt(v)
			if !ok || n < 0 || n >= Chess960Count {
				return fmt.Errorf("номер расстановки должен быть целым числом от 0 до %d", Chess960Count-1)
			}
			params["position"] = n
		case "number": // computed below; a rematch brings the old one along
		default:
			return fmt.Errorf("у Chess960 нет настройки %q", k)
		}
	}
	o.Params = params
	if !fixed {
		delete(o.Params, "position")
	}
	o.Params["number"] = chess960Number(*o)
	return nil
}

func init() { Register(chess960{}) }

package variants

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Blindfold chess: the rules are the classical ones, but the pieces are not
// shown. While the game is on, nobody's view carries the position or the
// check; the view keeps the two squares of the last move (the browser lights
// them on the empty board). The players keep the position in their heads. The
// choice "peek" of the lobby lets the players look at the position for a few
// seconds (the browser times that); without it the position is not sent at all
// (Extra carries it only when peeking is allowed). When the game ends the view
// becomes the ordinary one.
//
// A move is an action: the browser sends the long form from the legal list
// ("g1f3"), or what a person typed ("Nf3", "O-O", "e2e4"), which blindState
// resolves against the legal moves and answers politely when it cannot.

type blindRules struct{ ClassicRules }

// blindExtra is the Extra of a blindfold game.
type blindExtra struct {
	// Blind: the position is hidden now (the game is on).
	Blind bool `json:"blind"`
	// Peek: the lobby allowed looking at the position.
	Peek bool `json:"peek"`
	// Board is the position, given only while it is hidden and peeking is allowed.
	Board map[string]string `json:"board,omitempty"`
}

func peekAllowed(o Options) bool {
	b, _ := o.Params["peek"].(bool)
	return b
}

func (blindRules) Extra(g *Game, _ Side) any {
	_, _, over := g.Over()
	e := blindExtra{Blind: !over, Peek: peekAllowed(g.Options())}
	if e.Blind && e.Peek {
		e.Board = g.Position().Board()
	}
	return e
}

func (blindRules) Redact(g *Game, _ Side, v *View) {
	if _, _, over := g.Over(); over {
		return
	}
	// The squares of the last move stay for everybody (players and spectators): they say where the
	// move went, not what stands on the board. The check is hidden (it would name the king's square).
	// Castling shows the king's two squares, en passant and promotion the pawn's two.
	v.Board = map[string]string{}
	v.Check = nil
}

// blindState is a Game that also reads moves typed by hand.
type blindState struct{ *Game }

func (s *blindState) Apply(action string) (string, error) {
	if _, _, over := s.Game.Over(); over {
		return "", ErrOver
	}
	action = strings.TrimSpace(action)
	if _, ok := Find(s.Game.rules, s.Game.Moves(), action); ok {
		return s.Game.Apply(action)
	}
	mv, err := ResolveTyped(s.Game.rules, s.Game.Position(), s.Game.Moves(), action)
	if err != nil {
		return "", err
	}
	return s.Game.Apply(Encode(s.Game.rules, mv))
}

type blindfold struct{}

func (blindfold) Info() Info {
	return Info{
		ID:    "blindfold",
		Name:  "Слепые шахматы",
		Short: "Обычные правила, но фигур на доске не видно: позицию держите в голове, ходы вводите текстом или кликами по клеткам.",
	}
}

func (blindfold) New(o Options) (State, error) {
	g, err := NewGame(blindRules{}, o)
	if err != nil {
		return nil, err
	}
	return &blindState{Game: g}, nil
}

// CheckOptions: "peek" (true or false, false by default).
func (blindfold) CheckOptions(o *Options) error {
	peek := false
	for k, v := range o.Params {
		if k != "peek" {
			return fmt.Errorf("у слепых шахмат нет настройки %q", k)
		}
		b, ok := v.(bool)
		if !ok {
			return errors.New("«подглядывание» может быть только включено или выключено")
		}
		peek = b
	}
	o.Params = map[string]any{"peek": peek}
	return nil
}

func init() { Register(blindfold{}) }

// ------------------------------------------------------------ typed moves

// typedMove: an optional piece letter, an optional file and rank of the square
// it stands on, an optional "x" or "-", the target square, an optional
// promotion ("e8=Q", "e8Q", "e7e8q").
var typedMove = regexp.MustCompile(`^([KQRBNkqrn])?([a-h])?([1-8])?[-x:]?([a-h])([1-8])(?:=?([QRBNqrbn]))?$`)

var typedCastle = regexp.MustCompile(`^[oO0]-[oO0](-[oO0])?$`)

const typedHelp = "Примеры: Nf3, e4, exd5, e8=Q, O-O, O-O-O или e2e4."

// ResolveTyped finds the legal move a typed text means. The text is the
// algebraic notation (Nf3, Nbd2, exd5, e8=Q, O-O) or the long form (e2e4,
// Ng1f3, e7e8q); capital letters are the pieces, a lower-case b is a file; case
// and spaces are forgiven where nothing is lost. The error says, in words for
// the player, what was wrong.
func ResolveTyped(r Rules, p *Position, legal []Move, text string) (Move, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Move{}, errors.New("Введите ход. " + typedHelp)
	}
	for _, c := range text {
		if unicode.Is(unicode.Cyrillic, c) {
			return Move{}, errors.New("Записывайте ход латиницей: N конь, B слон, R ладья, Q ферзь, K король, O-O рокировка. " + typedHelp)
		}
	}
	if len(text) > 16 {
		return Move{}, errors.New("Не понял запись хода. " + typedHelp)
	}
	text = strings.Join(strings.Fields(text), "")
	text = strings.TrimRight(text, "+#!?")

	if typedCastle.MatchString(text) {
		long := strings.Count(text, "-") == 2
		for _, mv := range legal {
			if mv.Castle && (mv.To.File() == 2) == long {
				return mv, nil
			}
		}
		if long {
			return Move{}, errors.New("Длинная рокировка сейчас невозможна.")
		}
		return Move{}, errors.New("Короткая рокировка сейчас невозможна.")
	}

	// The spellings to try: as typed; a leading lower-case b read as a bishop;
	// everything in lower case ("E2E4", "NF3").
	tries := []string{text}
	if text[0] == 'b' {
		tries = append(tries, "B"+text[1:])
	}
	if lower := strings.ToLower(text); lower != text {
		tries = append(tries, lower)
	}
	parsed := false
	var ambiguous []Move
	for _, t := range tries {
		m := typedMove.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		parsed = true
		found, err := matchTyped(p, legal, m)
		if err == nil && len(found) == 1 {
			return found[0], nil
		}
		if len(found) > 1 {
			ambiguous = found
			break
		}
	}
	if !parsed {
		return Move{}, fmt.Errorf("Не понял запись «%s». %s", text, typedHelp)
	}
	if len(ambiguous) > 1 {
		return Move{}, ambiguityError(r, p, legal, text, ambiguous)
	}
	return Move{}, fmt.Errorf("Хода «%s» сейчас нет. Проверьте запись и положение фигур.", text)
}

// matchTyped lists the legal moves the parsed groups of typedMove fit.
func matchTyped(p *Position, legal []Move, m []string) ([]Move, error) {
	kind := NoKind
	if m[1] != "" {
		kind = KindOf(m[1][0])
	}
	ff, fr := -1, -1
	if m[2] != "" {
		ff = int(m[2][0] - 'a')
	}
	if m[3] != "" {
		fr = int(m[3][0] - '1')
	}
	to, _ := ParseSq(m[4] + m[5])
	promo := NoKind
	if m[6] != "" {
		promo = KindOf(m[6][0])
	}
	var out []Move
	for _, mv := range legal {
		if mv.Drop != NoKind || mv.To != to {
			continue
		}
		pc := p.B[mv.From]
		switch {
		case kind != NoKind:
			if pc.Kind != kind {
				continue
			}
		case ff >= 0 && fr >= 0:
			// the whole starting square is named: any piece
		default:
			// a pawn; a push has no file in front of it
			if pc.Kind != Pawn || ff < 0 && mv.From.File() != mv.To.File() {
				continue
			}
		}
		if ff >= 0 && mv.From.File() != ff || fr >= 0 && mv.From.Rank() != fr {
			continue
		}
		if mv.Promo != NoKind && promo != NoKind && mv.Promo != promo || mv.Promo == NoKind && promo != NoKind {
			continue
		}
		out = append(out, mv)
	}
	return out, nil
}

func ambiguityError(r Rules, p *Position, legal []Move, text string, found []Move) error {
	// a promotion without its piece: the moves differ only in what the pawn becomes
	same := true
	for _, mv := range found[1:] {
		if mv.From != found[0].From || mv.To != found[0].To {
			same = false
		}
	}
	if same && found[0].Promo != NoKind {
		return errors.New("Для превращения допишите фигуру, например e8=Q (Q ферзь, R ладья, B слон, N конь).")
	}
	var names []string
	for _, mv := range found {
		names = append(names, SAN(r, p, legal, mv, p.Apply(r, mv)))
	}
	return fmt.Errorf("Уточните ход «%s»: подходят %s.", text, strings.Join(names, " и "))
}

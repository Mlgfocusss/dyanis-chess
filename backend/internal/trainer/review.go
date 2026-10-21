package trainer

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/stats"
	"dyanis-chess/backend/internal/store"
)

// ---------------------------------------------------------------- phases

// Phase is a stage of a game.
type Phase string

const (
	PhaseOpening    Phase = "opening"
	PhaseMiddlegame Phase = "middlegame"
	PhaseEndgame    Phase = "endgame"
)

// Phases lists the stages in the order a game goes through them.
var Phases = []Phase{PhaseOpening, PhaseMiddlegame, PhaseEndgame}

// PhaseName is the Russian name of a stage.
func PhaseName(p Phase) string {
	switch p {
	case PhaseOpening:
		return "Дебют"
	case PhaseMiddlegame:
		return "Миттельшпиль"
	}
	return "Эндшпиль"
}

// Material is the worth of the pieces on a board given by its FEN, both sides together: knight and
// bishop 3, rook 5, queen 9. Pawns and kings are not counted.
func Material(fen string) int {
	placement, _, _ := strings.Cut(fen, " ")
	total := 0
	for _, c := range placement {
		switch c {
		case 'n', 'N', 'b', 'B':
			total += 3
		case 'r', 'R':
			total += 5
		case 'q', 'Q':
			total += 9
		}
	}
	return total
}

// PhasesOf tells the stage of every half-move of a game. fens[i] is the position before half-move
// i+1 and book[i] says whether that move is theory. A game is in its opening up to OpeningPlies, and
// as long as it is still in theory from the first move; after that it is a middlegame until the
// material falls to EndgameMaterial, and an endgame from then on (the material is looked at
// before the move, and once the endgame has begun it stays one, even if a pawn queens).
func PhasesOf(fens []string, book []bool) []Phase {
	bookRun := 0
	for bookRun < len(book) && book[bookRun] {
		bookRun++
	}
	openingEnd := max(OpeningPlies, bookRun)
	out := make([]Phase, len(book))
	endgame := false
	for i := range book {
		ply := i + 1
		switch {
		case ply <= openingEnd:
			out[i] = PhaseOpening
			continue
		case i < len(fens) && Material(fens[i]) <= EndgameMaterial:
			endgame = true
		}
		if endgame {
			out[i] = PhaseEndgame
		} else {
			out[i] = PhaseMiddlegame
		}
	}
	return out
}

// ---------------------------------------------------------------- one review

// OwnMove is one move of the person in a reviewed game.
type OwnMove struct {
	Ply      int
	SAN      string
	Best     string
	Class    string
	Loss     float64
	Accuracy float64
	Book     bool
	Phase    Phase
}

// Review is one reviewed game from the person's side.
type Review struct {
	ID       string
	At       time.Time
	Side     string // "white" or "black"
	Accuracy float64
	Opponent string
	Moves    []OwnMove
}

// Why a review is left out of the picture.
const (
	SkipColour     = "colour"     // the person's name is not (or twice) in the game's players
	SkipUnreadable = "unreadable" // the stored report cannot be read
)

type reportDoc struct {
	Moves []struct {
		Ply      int     `json:"ply"`
		Color    string  `json:"color"`
		SAN      string  `json:"san"`
		Class    string  `json:"class"`
		Loss     float64 `json:"loss"`
		Accuracy float64 `json:"accuracy"`
		BestSAN  string  `json:"bestSan"`
		Book     bool    `json:"book"`
	} `json:"moves"`
	Evals []struct {
		FEN string `json:"fen"`
	} `json:"evals"`
	Accuracy [2]float64 `json:"accuracy"`
}

// ReadReview reads a stored review from the point of view of the person called name. The second
// result is empty when the review is usable, otherwise the reason it is skipped (Skip...).
func ReadReview(row store.ReportRow, name string) (*Review, string) {
	var game struct {
		White string `json:"white"`
		Black string `json:"black"`
	}
	var doc reportDoc
	if json.Unmarshal(row.Game, &game) != nil || json.Unmarshal(row.Report, &doc) != nil || len(doc.Moves) == 0 {
		return nil, SkipUnreadable
	}
	side := stats.SideOf(name, game.White, game.Black)
	if side == "both" {
		return nil, SkipColour
	}
	r := &Review{ID: row.ID, At: row.CreatedAt, Side: side, Opponent: game.Black}
	r.Accuracy = doc.Accuracy[0]
	if side == "black" {
		r.Accuracy, r.Opponent = doc.Accuracy[1], game.White
	}

	fens := make([]string, len(doc.Evals))
	for i, e := range doc.Evals {
		fens[i] = e.FEN
	}
	book := make([]bool, len(doc.Moves))
	for i, m := range doc.Moves {
		book[i] = m.Book || m.Class == string(analysis.Book)
	}
	phase := PhasesOf(fens, book)
	for i, m := range doc.Moves {
		if m.Color != side {
			continue
		}
		r.Moves = append(r.Moves, OwnMove{Ply: m.Ply, SAN: m.SAN, Best: m.BestSAN, Class: m.Class, Loss: m.Loss,
			Accuracy: m.Accuracy, Book: book[i], Phase: phase[i]})
	}
	return r, ""
}

// Severity says whether a move is an "inaccuracy", a "mistake" or a "blunder" ("" for any other).
// A "miss" (a lost chance) is judged by its loss.
func Severity(class string, loss float64) string {
	switch class {
	case string(analysis.Inaccuracy), string(analysis.Mistake), string(analysis.Blunder):
		return class
	case string(analysis.Miss):
		switch {
		case loss >= BlunderLoss:
			return string(analysis.Blunder)
		case loss >= MistakeLoss:
			return string(analysis.Mistake)
		}
		return string(analysis.Inaccuracy)
	}
	return ""
}

// ---------------------------------------------------------------- the picture

// PhaseStat is the accuracy in one stage of the games; Accuracy is nil when there are too few moves.
type PhaseStat struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Accuracy *float64 `json:"accuracy"`
	Moves    int      `json:"moves"`
}

// Rate is how many slips the person makes per 100 of their moves.
type Rate struct {
	Moves        int     `json:"moves"`
	Inaccuracies float64 `json:"inaccuracies"`
	Mistakes     float64 `json:"mistakes"`
	Blunders     float64 `json:"blunders"`
}

// Trend compares the latest reviews with the ones before them.
type Trend struct {
	Recent        float64 `json:"recent"`
	Previous      float64 `json:"previous"`
	Delta         float64 `json:"delta"`
	RecentCount   int     `json:"recentCount"`
	PreviousCount int     `json:"previousCount"`
	Text          string  `json:"text"` // "+3,2 пункта"
}

// Point is the accuracy of one review in the chart.
type Point struct {
	ID       string    `json:"id"`
	Accuracy float64   `json:"accuracy"`
	At       time.Time `json:"at"`
}

// Costly is one of the moves that cost the most.
type Costly struct {
	Review   string    `json:"review"`
	Ply      int       `json:"ply"`
	Number   int       `json:"number"` // the move number: 7 for 7.Ng5 and 7...Nf6
	Black    bool      `json:"black"`
	SAN      string    `json:"san"`
	Best     string    `json:"best,omitempty"`
	Class    string    `json:"class"`
	Loss     float64   `json:"loss"`
	Opponent string    `json:"opponent,omitempty"`
	At       time.Time `json:"at"`
	Href     string    `json:"href"`
}

// Insights is the answer to "how do you play", from the reviews.
type Insights struct {
	// Reviews is how many of the latest reviews were read, Used how many of them have the person's
	// colour recognised and went into the numbers, Skipped the rest.
	Reviews int `json:"reviews"`
	Used    int `json:"used"`
	Skipped int `json:"skipped"`
	// Enough says whether there are MinReviews of them; without it the page shows the invitation.
	Enough  bool        `json:"enough"`
	Need    int         `json:"need"`
	Average *float64    `json:"average"`
	Phases  []PhaseStat `json:"phases"`
	Rate    Rate        `json:"rate"`
	Trend   *Trend      `json:"trend"`
	Series  []Point     `json:"series"` // oldest first
	Costly  []Costly    `json:"costly"`
	// Rule says how the stages are told apart (the page prints it).
	Rule string `json:"rule"`
}

// PhaseRule is the sentence that explains the stages to the person.
func PhaseRule() string {
	return fmt.Sprintf("Дебют: до %d-го полухода и пока вы играете по теории. Эндшпиль: когда на доске осталось фигур не больше чем на %d очков (конь и слон по 3, ладья 5, ферзь 9, у обоих вместе; пешки не считаются). Всё между ними миттельшпиль. Ходы по теории в точность не входят.",
		OpeningPlies, EndgameMaterial)
}

// BuildInsights puts reviews (newest first) together. rows is how many reviews were read and
// skipped how many of those were left out (see ReadReview).
func BuildInsights(reviews []*Review, rows, skipped int) Insights {
	in := Insights{Reviews: rows, Used: len(reviews), Skipped: skipped, Enough: len(reviews) >= MinReviews, Need: MinReviews,
		Phases: []PhaseStat{}, Series: []Point{}, Costly: []Costly{}, Rule: PhaseRule()}
	if len(reviews) == 0 {
		for _, p := range Phases {
			in.Phases = append(in.Phases, PhaseStat{ID: string(p), Name: PhaseName(p)})
		}
		return in
	}

	sum := 0.0
	for _, r := range reviews {
		sum += r.Accuracy
	}
	avg := round1(sum / float64(len(reviews)))
	in.Average = &avg
	for i := len(reviews) - 1; i >= 0; i-- { // oldest first
		in.Series = append(in.Series, Point{ID: reviews[i].ID, Accuracy: round1(reviews[i].Accuracy), At: reviews[i].At})
	}
	in.Trend = TrendOf(reviews)

	// stages and slips over all the moves of all the reviews
	type acc struct {
		sum   float64
		moves int
	}
	byPhase := map[Phase]*acc{}
	for _, p := range Phases {
		byPhase[p] = &acc{}
	}
	var count [3]int // inaccuracies, mistakes, blunders
	var all []Costly
	for _, r := range reviews {
		for _, m := range r.Moves {
			in.Rate.Moves++
			if !m.Book {
				a := byPhase[m.Phase]
				a.sum += m.Accuracy
				a.moves++
			}
			sev := Severity(m.Class, m.Loss)
			switch sev {
			case "inaccuracy":
				count[0]++
			case "mistake":
				count[1]++
			case "blunder":
				count[2]++
			}
			if sev == "mistake" || sev == "blunder" || m.Loss >= CostlyMinLoss {
				all = append(all, Costly{Review: r.ID, Ply: m.Ply, Number: (m.Ply + 1) / 2, Black: m.Ply%2 == 0, SAN: m.SAN, Best: m.Best,
					Class: sev, Loss: round1(m.Loss), Opponent: r.Opponent, At: r.At, Href: "#review/" + r.ID})
			}
		}
	}
	for _, p := range Phases {
		a := byPhase[p]
		st := PhaseStat{ID: string(p), Name: PhaseName(p), Moves: a.moves}
		if a.moves >= MinPhaseMoves {
			v := round1(a.sum / float64(a.moves))
			st.Accuracy = &v
		}
		in.Phases = append(in.Phases, st)
	}
	if in.Rate.Moves > 0 {
		per100 := func(n int) float64 { return round1(100 * float64(n) / float64(in.Rate.Moves)) }
		in.Rate.Inaccuracies, in.Rate.Mistakes, in.Rate.Blunders = per100(count[0]), per100(count[1]), per100(count[2])
	}

	// the dearest moves first; among equal ones the newer
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Loss != all[j].Loss {
			return all[i].Loss > all[j].Loss
		}
		return all[i].At.After(all[j].At)
	})
	if len(all) > CostlyTop {
		all = all[:CostlyTop]
	}
	in.Costly = append(in.Costly, all...)
	return in
}

// TrendOf compares the accuracy of the latest reviews with the ones before them (reviews are newest
// first): the newest TrendWindow against the TrendWindow before them, and with fewer than twice as many
// reviews the newest half against the older half. nil when either group has fewer than TrendMin.
func TrendOf(reviews []*Review) *Trend {
	n := min(TrendWindow, (len(reviews)+1)/2)
	recent := reviews[:n]
	rest := reviews[n:]
	previous := rest[:min(TrendWindow, len(rest))]
	if len(recent) < TrendMin || len(previous) < TrendMin {
		return nil
	}
	mean := func(rs []*Review) float64 {
		s := 0.0
		for _, r := range rs {
			s += r.Accuracy
		}
		return s / float64(len(rs))
	}
	t := &Trend{Recent: round1(mean(recent)), Previous: round1(mean(previous)), RecentCount: len(recent), PreviousCount: len(previous)}
	t.Delta = round1(mean(recent) - mean(previous))
	t.Text = PointsText(t.Delta)
	return t
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }

// ---------------------------------------------------------------- words

// Num writes a number the Russian way, with a decimal comma and without a trailing ",0".
func Num(x float64) string {
	s := fmt.Sprintf("%.1f", x)
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1)
}

// PointsText writes a change of accuracy: "+3,2 пункта", "−1 пункт", "без изменений".
func PointsText(delta float64) string {
	d := round1(delta)
	if d == 0 {
		return "без изменений"
	}
	sign := "+"
	if d < 0 {
		sign = "−"
	}
	a := math.Abs(d)
	word := "пункта" // a fraction always takes the genitive singular
	if a == math.Trunc(a) {
		word = Plural(int(a), "пункт", "пункта", "пунктов")
	}
	return sign + Num(a) + " " + word
}

// Plural picks the form of a noun after a number: 1 день, 2 дня, 5 дней.
func Plural(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	m10, m100 := n%10, n%100
	switch {
	case m10 == 1 && m100 != 11:
		return one
	case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
		return few
	}
	return many
}

// MoveLabel writes a move with its number the way a book does: "7.Ng5" and "7...Nf6".
func MoveLabel(c Costly) string {
	if c.Black {
		return fmt.Sprintf("%d...%s", c.Number, c.SAN)
	}
	return fmt.Sprintf("%d.%s", c.Number, c.SAN)
}

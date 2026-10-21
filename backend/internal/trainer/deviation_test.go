package trainer

import (
	"testing"
	"time"

	"dyanis-chess/backend/internal/store"
)

// line makes the moves of a repository's tree: each UCI follows the one before it; the person's own
// moves are those of the colour whose turn it is by the ply.
func line(color string, first int64, ucis ...string) []store.RepMove {
	var out []store.RepMove
	parent := int64(0)
	for i, u := range ucis {
		ply := i + 1
		mine := (ply%2 == 1) == (color == "white")
		id := first + int64(i)
		out = append(out, store.RepMove{ID: id, ParentID: parent, UCI: u, SAN: u, Ply: ply, Mine: mine})
		parent = id
	}
	return out
}

func TestDeviationOf(t *testing.T) {
	italian := RepoMoves{Color: "white", Moves: line("white", 1, "e2e4", "e7e5", "g1f3", "b8c6")}
	// a second repository of the same colour: 1.e4 c5
	sicilian := RepoMoves{Color: "white", Moves: line("white", 10, "e2e4", "c7c5", "g1f3")}
	blackRep := RepoMoves{Color: "black", Moves: line("black", 20, "e2e4", "c7c5")}
	empty := RepoMoves{Color: "white"}

	games := []GameMoves{
		{Color: "white", UCIs: []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1c4"}}, // the Italian to its end: fine
		{Color: "white", UCIs: []string{"e2e4", "c7c5", "g1f3"}},                 // the Sicilian: fine, though the Italian is left by the opponent
		{Color: "white", UCIs: []string{"d2d4", "d7d5"}},                         // neither repository: a deviation
		{Color: "white", UCIs: []string{"e2e4", "e7e5", "f1c4"}},                 // left the Italian at the person’s own move, but the Sicilian expected another reply from the opponent: fine
		{Color: "white", UCIs: []string{"e2e4", "d7d6"}},                         // the opponent left both: fine
		{Color: "black", UCIs: []string{"e2e4", "c7c5", "g1f3"}},                 // Black's repository: in book
		{Color: "black", UCIs: []string{"d2d4", "g8f6"}},                         // the opponent left it: fine
	}
	d := DeviationOf([]RepoMoves{italian, sicilian, blackRep, empty}, games)
	if d.Compared != 7 || d.Deviated != 1 {
		t.Errorf("deviation: %+v", d)
	}

	// without a repository of the colour a game says nothing
	if d := DeviationOf([]RepoMoves{blackRep}, games[:5]); d != (Deviation{}) {
		t.Errorf("no white repository: %+v", d)
	}
	// an empty repository and a game that cannot be replayed are left out
	if d := DeviationOf([]RepoMoves{empty, italian}, []GameMoves{{Color: "white", UCIs: []string{"e2e4"}}, {Color: "white", UCIs: []string{"e2e5"}}}); d.Compared != 1 || d.Deviated != 0 {
		t.Errorf("broken: %+v", d)
	}
	if d := DeviationOf(nil, games); d != (Deviation{}) {
		t.Errorf("nothing: %+v", d)
	}
}

func TestReadGames(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	state := func(white, black, bot string, moves int) []byte {
		ms := ""
		for i := 0; i < moves; i++ {
			if i > 0 {
				ms += ","
			}
			ms += `{"uci":"a2a3"}`
		}
		return []byte(`{"white":{"name":"` + white + `"},"black":{"name":"` + black + `","bot":"` + bot + `"},"moves":[` + ms + `]}`)
	}
	rows := []*store.Game{
		{ID: "win", WhiteID: "me", BlackID: "", Status: "ended", Result: "1-0", State: state("Я", "Лиза", "liza", 12), CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "loss", WhiteID: "other", BlackID: "me", Status: "ended", Result: "1-0", State: []byte(`{"white":{"name":"Борис"},"black":{"name":"Я"},"moves":[{"uci":"a2a3"},{"uci":"a7a6"},{"uci":"b2b3"},{"uci":"b7b6"}]}`), CreatedAt: now.Add(-30 * time.Hour), UpdatedAt: now.Add(-29 * time.Hour)},
		{ID: "live", WhiteID: "me", Status: "playing", State: state("Я", "Лиза", "liza", 12), UpdatedAt: now},
		{ID: "short", WhiteID: "me", Status: "ended", Result: "1-0", State: state("Я", "Лиза", "liza", 2), UpdatedAt: now},
		{ID: "old", WhiteID: "me", Status: "ended", Result: "1/2-1/2", State: state("Я", "Лиза", "liza", 12), UpdatedAt: now.AddDate(0, 0, -9)},
		{ID: "broken", WhiteID: "me", Status: "ended", Result: "1-0", State: []byte(`{`), UpdatedAt: now},
		{ID: "foreign", WhiteID: "a", BlackID: "b", Status: "ended", Result: "1-0", State: state("A", "B", "", 12), UpdatedAt: now},
	}
	got := ReadGames(rows, "me", now.AddDate(0, 0, -7), MinReviewPlies)
	if len(got) != 2 || got[0].ID != "win" || got[1].ID != "loss" {
		t.Fatalf("games: %+v", got)
	}
	w, l := got[0], got[1]
	if w.Color != "white" || w.Result != "win" || w.Opponent != "Лиза" || !w.Bot || len(w.UCIs) != 12 || w.WhiteName != "Я" {
		t.Errorf("win: %+v", w)
	}
	if l.Color != "black" || l.Result != "loss" || l.Opponent != "Борис" || l.Bot {
		t.Errorf("loss: %+v", l)
	}
	// the old one comes back with a longer look-back
	if got := ReadGames(rows, "me", now.AddDate(0, 0, -30), MinReviewPlies); len(got) != 3 || got[2].Result != "draw" {
		t.Errorf("30 days: %+v", got)
	}
}

func TestReviewed(t *testing.T) {
	began := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	g := Seen{ID: "g", WhiteName: "Анна", BlackName: "Лиза", CreatedAt: began}
	infos := []ReviewInfo{
		{White: "Анна", Black: "Лиза", At: began.Add(-time.Hour)}, // an earlier game against the same player
		{White: "Борис", Black: "Лиза", At: began.Add(time.Hour)}, // another game
	}
	if Reviewed(g, infos) {
		t.Errorf("neither review is of this game")
	}
	infos = append(infos, ReviewInfo{White: "Анна", Black: "Лиза", At: began.Add(2 * time.Hour)})
	if !Reviewed(g, infos) {
		t.Errorf("the later review is of this game")
	}
	// names the PGN writer changes (quotes) and a missing seat
	odd := Seen{WhiteName: `Ан"на`, BlackName: "", CreatedAt: began}
	if !Reviewed(odd, []ReviewInfo{{White: "Ан'на", Black: "?", At: began}}) {
		t.Errorf("sanitised names")
	}
}

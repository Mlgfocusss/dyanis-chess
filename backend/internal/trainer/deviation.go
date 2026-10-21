package trainer

import (
	"encoding/json"
	"time"

	"dyanis-chess/backend/internal/repertoire"
	"dyanis-chess/backend/internal/store"
)

// Deviation counts how often the person's recent games left their repositories of moves.
type Deviation struct {
	// Compared is the number of games that could be put against a repository of the same colour.
	Compared int `json:"compared"`
	// Deviated is how many of them left every such repository at the person's own move.
	Deviated int `json:"deviated"`
}

// RepoMoves is a repository with its tree.
type RepoMoves struct {
	Color string // "white" or "black"
	Moves []store.RepMove
}

// GameMoves is a game of the person with its moves (UCI).
type GameMoves struct {
	Color string
	UCIs  []string
}

// DeviationOf puts games against repositories of the same colour. A game that stayed on the line of
// some repository, or left it at the opponent's move or where the repository simply ends, is fine;
// it counts as a deviation only when it left every repository of its colour at the person's own move
// (a person with an Italian and a Sicilian repository plays only one of them in a game). Games that
// no repository has anything to say about (empty ones, unreadable moves) are not counted at all.
func DeviationOf(repos []RepoMoves, games []GameMoves) Deviation {
	var d Deviation
	for _, g := range games {
		compared, fine := 0, false
		for _, r := range repos {
			if r.Color != g.Color {
				continue
			}
			switch repertoire.Compare(r.Moves, r.Color, g.UCIs).Status {
			case repertoire.StatusEmpty, repertoire.StatusBroken:
				continue
			case repertoire.StatusYouDeviated:
				compared++
			default:
				compared++
				fine = true
			}
		}
		if compared == 0 {
			continue
		}
		d.Compared++
		if !fine {
			d.Deviated++
		}
	}
	return d
}

// savedGame is the part of an online game's stored state that matters here (see internal/play).
type savedGame struct {
	Moves []struct {
		UCI string `json:"uci"`
	} `json:"moves"`
	White *struct {
		Name string `json:"name"`
		Bot  string `json:"bot"`
	} `json:"white"`
	Black *struct {
		Name string `json:"name"`
		Bot  string `json:"bot"`
	} `json:"black"`
}

// Seen is a game of the person read from the games table.
type Seen struct {
	ID                   string
	Color                string
	Opponent             string
	Bot                  bool
	Result               string // "win", "loss", "draw" for the person
	At                   time.Time
	CreatedAt            time.Time
	WhiteName, BlackName string
	UCIs                 []string
}

// ReadGames reads the person's finished games with at least minPlies half-moves, newest first,
// dropping the ones that were last touched before since.
func ReadGames(rows []*store.Game, userID string, since time.Time, minPlies int) []Seen {
	var out []Seen
	for _, g := range rows {
		if g.Status != "ended" || g.UpdatedAt.Before(since) {
			continue
		}
		color := ""
		switch userID {
		case g.WhiteID:
			color = "white"
		case g.BlackID:
			color = "black"
		default:
			continue
		}
		var s savedGame
		if json.Unmarshal(g.State, &s) != nil || len(s.Moves) < minPlies {
			continue
		}
		seen := Seen{ID: g.ID, Color: color, At: g.UpdatedAt.UTC(), CreatedAt: g.CreatedAt.UTC(), Result: outcome(g.Result, color)}
		for _, m := range s.Moves {
			seen.UCIs = append(seen.UCIs, m.UCI)
		}
		if s.White != nil {
			seen.WhiteName = s.White.Name
		}
		if s.Black != nil {
			seen.BlackName = s.Black.Name
		}
		them := s.Black
		if color == "black" {
			them = s.White
		}
		if them != nil {
			seen.Opponent, seen.Bot = them.Name, them.Bot != ""
		}
		out = append(out, seen)
	}
	return out
}

// outcome turns a stored result into win, loss or draw for the side.
func outcome(result, color string) string {
	switch {
	case result == "1/2-1/2":
		return "draw"
	case result == "1-0" && color == "white", result == "0-1" && color == "black":
		return "win"
	case result == "1-0" || result == "0-1":
		return "loss"
	}
	return "draw"
}

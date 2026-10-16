package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"dyanis-chess/backend/internal/eco"
	"dyanis-chess/engine"
)

// The "what is played here" endpoint of the repertoire builder: the moves from a
// position that opening theory knows, taken from the embedded ECO reference
// (the same book that names the openings of a review). The engine's verdict on
// each is asked for separately, through /api/line, so that the list appears at
// once and the evaluations fill in.

type continuationsRequest struct {
	FEN string `json:"fen"`
}

// Continuation is one move that leads along a line of the reference.
type Continuation struct {
	UCI string `json:"uci"`
	SAN string `json:"san"`
	// Opening names the position the move reaches, when the reference names it.
	Opening *eco.Opening `json:"opening,omitempty"`
	// Lines is how many reference lines pass through the position the move
	// reaches: the more, the more is known about it (and the more often it is played).
	Lines int `json:"lines"`
}

type continuationsResponse struct {
	// Book says that the position itself is on a reference line.
	Book  bool           `json:"book"`
	Moves []Continuation `json:"moves"`
}

func (s *Server) continuations(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<10)
	var req continuationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	pos, err := engine.ParseFEN(req.FEN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверная позиция")
		return
	}
	writeJSON(w, http.StatusOK, TheoryMoves(eco.Default(), pos))
}

// TheoryMoves lists the moves of pos that lead to a position on a line of the book.
func TheoryMoves(book *eco.Book, pos *engine.Position) continuationsResponse {
	out := continuationsResponse{Book: book.InBook(pos.Hash()), Moves: []Continuation{}}
	for _, m := range pos.LegalMoves() {
		c := pos.Copy()
		c.Play(m)
		h := c.Hash()
		if !book.InBook(h) {
			continue
		}
		cont := Continuation{UCI: m.String(), SAN: pos.SAN(m), Lines: book.Lines(h)}
		if o, ok := book.Lookup(h); ok {
			o := o
			cont.Opening = &o
		}
		out.Moves = append(out.Moves, cont)
	}
	// the best known first, then by name so that the order is stable
	sort.SliceStable(out.Moves, func(i, j int) bool {
		if out.Moves[i].Lines != out.Moves[j].Lines {
			return out.Moves[i].Lines > out.Moves[j].Lines
		}
		return out.Moves[i].SAN < out.Moves[j].SAN
	})
	return out
}

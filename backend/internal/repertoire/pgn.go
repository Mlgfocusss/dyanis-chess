package repertoire

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"dyanis-chess/backend/internal/pgn"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

// Repertoires travel as PGN: a file holds one game per chapter, the variations are the branches
// of the tree, and the colour is the "Orientation" tag (as in Lichess studies).

// maxPGNBytes caps an imported file.
const maxPGNBytes = 4 << 20

// ImportStats says what an import did.
type ImportStats struct {
	Games   int `json:"games"`   // games of the file that were read
	Skipped int `json:"skipped"` // games left out because they do not start from the initial position
	Added   int `json:"added"`   // moves new to the tree
	Moves   int `json:"moves"`   // moves in the tree afterwards
}

type importRequest struct {
	Name  string `json:"name"`  // for a new repertoire; empty: the Event tag of the file
	Color string `json:"color"` // "white" or "black"; empty: the Orientation tag of the file
	PGN   string `json:"pgn"`
}

// colorOf is the colour the file says it is for: the Orientation tag of the first game that has one.
func colorOf(trees []*pgn.Tree) string {
	for _, t := range trees {
		switch strings.ToLower(strings.TrimSpace(t.Tags["Orientation"])) {
		case "white":
			return "white"
		case "black":
			return "black"
		}
	}
	return ""
}

// readPGN parses the text of an import; the error is meant for the person.
func readPGN(text string) ([]*pgn.Tree, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("файл пуст")
	}
	trees, err := pgn.ParseTreesString(text)
	var pe *pgn.ParseError
	switch {
	case errors.As(err, &pe) && pe.Ply > 0:
		return nil, fmt.Errorf("в PGN ошибка: партия %d, полуход %d: %s", pe.Game, pe.Ply, pe.Msg)
	case errors.As(err, &pe):
		return nil, fmt.Errorf("в PGN ошибка: партия %d: %s", pe.Game, pe.Msg)
	case err != nil:
		return nil, errors.New("не удалось прочитать PGN")
	case len(trees) == 0:
		return nil, errors.New("в файле нет партий")
	}
	moves := 0
	for _, t := range trees {
		moves += t.Count()
	}
	if moves == 0 {
		return nil, errors.New("в файле нет ходов")
	}
	return trees, nil
}

// mergePGN adds the moves of the games to the repertoire's tree. A move the tree already has
// (same parent, same move) is not added again.
func (s *Service) mergePGN(r *http.Request, rep *store.Repertoire, trees []*pgn.Tree) (ImportStats, error) {
	ctx := r.Context()
	current, err := s.cfg.Store.RepMoves(ctx, rep.ID)
	if err != nil {
		return ImportStats{}, err
	}
	// ref names a move: a positive id is one in the tree, 0 the start, -(n+1) the n-th move of the batch.
	type key struct {
		parent int64
		uci    string
	}
	known := make(map[key]int64, len(current))
	for _, m := range current {
		known[key{m.ParentID, m.UCI}] = m.ID
	}
	var batch []store.NewRepMove
	var walk func(n *pgn.Node, parent int64, ply int)
	walk = func(n *pgn.Node, parent int64, ply int) {
		for _, c := range n.Children {
			uci := c.Move.String()
			k := key{parent, uci}
			ref, ok := known[k]
			if !ok {
				nm := store.NewRepMove{ParentID: parent, ParentIndex: -1, UCI: uci, SAN: c.SAN, FEN: c.FEN, Ply: ply + 1,
					Mine: ((ply+1)%2 == 1) == (rep.Color == "white")}
				if parent < 0 {
					nm.ParentID, nm.ParentIndex = 0, int(-parent-1)
				}
				batch = append(batch, nm)
				ref = -int64(len(batch))
				known[k] = ref
			}
			walk(c, ref, ply+1)
		}
	}
	st := ImportStats{}
	for _, t := range trees {
		if t.StartFEN != "" && t.StartFEN != engine.StartFEN {
			st.Skipped++
			continue
		}
		st.Games++
		walk(&t.Root, 0, 0)
	}
	if err := s.cfg.Store.AddRepMovesBulk(ctx, rep.ID, batch); err != nil {
		return ImportStats{}, err
	}
	st.Added, st.Moves = len(batch), len(current)+len(batch)
	return st, nil
}

func importError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusConflict, fmt.Sprintf("в репозитории будет больше %d ходов, это предел", store.MaxRepMoves))
		return
	}
	serverError(w, "import", err)
}

// importNew makes a new repertoire of a PGN file.
func (s *Service) importNew(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.user(w, r)
	if !ok {
		return
	}
	var req importRequest
	if err := decodeLarge(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос или файл слишком большой (до 4 МБ)")
		return
	}
	trees, err := readPGN(req.PGN)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	color := req.Color
	if color == "" {
		color = colorOf(trees)
	}
	if color != "white" && color != "black" {
		writeError(w, http.StatusBadRequest, "в файле не указан цвет (тег Orientation): выберите, за кого этот репозиторий")
		return
	}
	name := req.Name
	if strings.TrimSpace(name) == "" {
		for _, t := range trees {
			if e := t.Tags["Event"]; e != "" && e != "?" {
				name = e
				break
			}
		}
	}
	if strings.TrimSpace(name) == "" {
		name = "Импорт PGN"
	}
	name = strings.Join(strings.Fields(name), " ")
	if rs := []rune(name); len(rs) > 80 { // a long Event tag is cut, not refused
		name = string(rs[:80])
	}
	rep, err := s.cfg.Store.CreateRepertoire(r.Context(), uid, name, color)
	if errors.Is(err, store.ErrLimit) {
		writeError(w, http.StatusConflict, "слишком много репозиториев")
		return
	}
	if err != nil {
		serverError(w, "create", err)
		return
	}
	st, err := s.mergePGN(r, rep, trees)
	if err != nil {
		_ = s.cfg.Store.DeleteRepertoire(r.Context(), uid, rep.ID) // no half-made repertoire is left
		importError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"repertoire": RepertoireJSON{ID: rep.ID, Name: rep.Name, Color: rep.Color},
		"import":     st,
	})
}

// importInto adds a PGN file to an existing repertoire.
func (s *Service) importInto(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	var req importRequest
	if err := decodeLarge(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос или файл слишком большой (до 4 МБ)")
		return
	}
	trees, err := readPGN(req.PGN)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := s.mergePGN(r, rep, trees)
	if err != nil {
		importError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"import": st})
}

// export sends the repertoire as a PGN file: one game, the lines as variations.
func (s *Service) export(w http.ResponseWriter, r *http.Request) {
	rep, ok := s.owned(w, r)
	if !ok {
		return
	}
	moves, err := s.cfg.Store.RepMoves(r.Context(), rep.ID)
	if err != nil {
		serverError(w, "moves", err)
		return
	}
	text := ExportPGN(rep, moves)
	w.Header().Set("Content-Type", "application/x-chess-pgn; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="repertoire.pgn"; filename*=UTF-8''`+url.PathEscape(fileName(rep.Name))+".pgn")
	_, _ = w.Write([]byte(text))
}

// fileName makes a name safe to be a file's name.
func fileName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, name)
	if name = strings.TrimSpace(name); name == "" {
		return "repertoire"
	}
	return name
}

// ExportPGN writes a repertoire as one PGN game. The first move added at a position is the
// main line there; the others are variations.
func ExportPGN(rep *store.Repertoire, moves []store.RepMove) string {
	nodes := make(map[int64]*pgn.Node, len(moves))
	t := &pgn.Tree{
		Tags:     map[string]string{"Event": rep.Name, "Site": "dyanis-chess", "Orientation": rep.Color},
		TagOrder: []string{"Event", "Site", "Orientation"},
		StartFEN: engine.StartFEN,
		Result:   "*",
	}
	for _, m := range moves { // a move always comes after its parent
		n := &pgn.Node{SAN: m.SAN, FEN: m.FEN}
		nodes[m.ID] = n
		parent := &t.Root
		if m.ParentID != 0 {
			if p, ok := nodes[m.ParentID]; ok {
				parent = p
			} else {
				continue
			}
		}
		parent.Children = append(parent.Children, n)
	}
	return t.String()
}

func decodeLarge(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPGNBytes+(64<<10))).Decode(v)
}

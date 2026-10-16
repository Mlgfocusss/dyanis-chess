package repertoire

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func importBody(name, color, text string) string {
	b, _ := json.Marshal(map[string]string{"name": name, "color": color, "pgn": text})
	return string(b)
}

const italianPGN = `[Event "Мой репозиторий"]
[Orientation "white"]

1. e4 e5 (1... c5 2. Nf3) 2. Nf3 Nc6 (2... d6) 3. Bc4 *
`

func TestImportBuildsTheTree(t *testing.T) {
	a := newAPI(t)
	out := a.mustCall("POST", "/api/repertoires/import", importBody("", "", italianPGN), 201)
	rep := out["repertoire"].(map[string]any)
	st := out["import"].(map[string]any)
	if rep["name"] != "Мой репозиторий" || rep["color"] != "white" {
		t.Errorf("repertoire %v", rep)
	}
	// e4 e5 c5 Nf3(after c5) Nf3(after e5) Nc6 d6 Bc4
	if st["added"] != 8.0 || st["games"] != 1.0 || st["skipped"] != 0.0 || st["moves"] != 8.0 {
		t.Errorf("import %v", st)
	}
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	got := a.mustCall("GET", base, "", 200)
	counts := got["repertoire"].(map[string]any)["counts"].(map[string]any)
	// White's own moves: e4, both Nf3 and Bc4
	if counts["moves"] != 8.0 || counts["mine"] != 4.0 {
		t.Errorf("counts %v", counts)
	}
	// every move knows its parent: the tree has two roots of black's answers under e4
	roots, underE4 := 0, 0
	var e4 int64
	for _, m := range got["moves"].([]any) {
		mv := m.(map[string]any)
		if mv["parent"] == 0.0 {
			roots++
			e4 = id(mv["id"])
		}
	}
	for _, m := range got["moves"].([]any) {
		if mv := m.(map[string]any); id(mv["parent"]) == e4 {
			underE4++
		}
	}
	if roots != 1 || underE4 != 2 {
		t.Errorf("roots %d, answers to e4 %d", roots, underE4)
	}
}

func TestImportColour(t *testing.T) {
	a := newAPI(t)
	plain := "1. d4 d5 2. c4 *\n"
	if code, out := a.call("POST", "/api/repertoires/import", importBody("Ферзевый гамбит", "", plain)); code != 400 || !strings.Contains(fmt.Sprint(out["error"]), "Orientation") {
		t.Errorf("no colour anywhere: %d %v", code, out)
	}
	out := a.mustCall("POST", "/api/repertoires/import", importBody("Ферзевый гамбит", "black", plain), 201)
	if out["repertoire"].(map[string]any)["color"] != "black" {
		t.Errorf("%v", out)
	}
	// the person's choice beats the tag of the file
	out = a.mustCall("POST", "/api/repertoires/import", importBody("Другой", "black", italianPGN), 201)
	if out["repertoire"].(map[string]any)["color"] != "black" {
		t.Errorf("%v", out)
	}
	// with black, d4 is the opponent's move and d5 is mine
	base := fmt.Sprintf("/api/repertoires/%d", id(out["repertoire"].(map[string]any)["id"]))
	if c := a.mustCall("GET", base, "", 200)["repertoire"].(map[string]any)["counts"].(map[string]any); c["mine"] != 4.0 {
		t.Errorf("counts %v", c)
	}
}

func TestImportIntoMergesAndDoesNotRepeat(t *testing.T) {
	a := newAPI(t)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"Итальянка","color":"white"}`, 201)
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))
	m1 := a.mustCall("POST", base+"/moves", `{"parent":0,"uci":"e2e4"}`, 201)["move"].(map[string]any)

	st := a.mustCall("POST", base+"/import", importBody("", "", "1. e4 e5 2. Nf3 *\n"), 200)["import"].(map[string]any)
	if st["added"] != 2.0 || st["moves"] != 3.0 { // e4 was there
		t.Errorf("first import %v", st)
	}
	st = a.mustCall("POST", base+"/import", importBody("", "", "1. e4 e5 2. Nf3 *\n"), 200)["import"].(map[string]any)
	if st["added"] != 0.0 || st["moves"] != 3.0 {
		t.Errorf("the same file again %v", st)
	}
	st = a.mustCall("POST", base+"/import", importBody("", "", "1. e4 e5 2. Nf3 (2. Bc4) *\n"), 200)["import"].(map[string]any)
	if st["added"] != 1.0 || st["moves"] != 4.0 {
		t.Errorf("one new line %v", st)
	}
	// the move added by hand kept its identity (it is not replaced)
	got := a.mustCall("GET", base, "", 200)
	found := false
	for _, m := range got["moves"].([]any) {
		found = found || id(m.(map[string]any)["id"]) == id(m1["id"])
	}
	if !found {
		t.Error("the first move was replaced")
	}
	// somebody else's repertoire is not reachable
	a.user = "boris"
	a.mustCall("POST", base+"/import", importBody("", "", "1. e4 *\n"), 404)
	if code, _ := a.call("GET", base+"/export", ""); code != 404 {
		t.Errorf("export of somebody else's: %d", code)
	}
	a.user = ""
	a.mustCall("POST", "/api/repertoires/import", importBody("", "white", "1. e4 *\n"), 401)
}

func TestImportRefusals(t *testing.T) {
	a := newAPI(t)
	for name, c := range map[string]struct{ text, want string }{
		"empty":         {"  \n", "пуст"},
		"no moves":      {"[Event \"x\"]\n\n*\n", "нет ходов"},
		"not a game":    {"hello", "полуход 1"},
		"illegal move":  {"[Orientation \"white\"]\n\n1. e4 e5 2. Ke4 *\n", "полуход 3"},
		"bad variation": {"[Orientation \"white\"]\n\n1. e4 (1. e5) e5 *\n", "полуход 1"},
		"unclosed":      {"[Orientation \"white\"]\n\n1. e4 (1. d4 *\n", "партия 1"},
	} {
		code, out := a.call("POST", "/api/repertoires/import", importBody("x", "", c.text))
		if code != 400 || !strings.Contains(fmt.Sprint(out["error"]), c.want) {
			t.Errorf("%s: %d %v, want a message with %q", name, code, out, c.want)
		}
	}
	a.mustCall("POST", "/api/repertoires/import", `not json`, 400)
	// a refused file leaves no repertoire behind
	if list := a.mustCall("GET", "/api/repertoires", "", 200)["repertoires"].([]any); len(list) != 0 {
		t.Errorf("repertoires left: %v", list)
	}
}

func TestImportSkipsGamesFromOtherPositions(t *testing.T) {
	a := newAPI(t)
	text := `[Orientation "white"]

1. e4 e5 *

[Orientation "white"]
[SetUp "1"]
[FEN "4k3/8/8/8/8/8/4P3/4K3 w - - 0 1"]

1. e4 *
`
	st := a.mustCall("POST", "/api/repertoires/import", importBody("Два", "", text), 201)["import"].(map[string]any)
	if st["games"] != 1.0 || st["skipped"] != 1.0 || st["added"] != 2.0 {
		t.Errorf("%v", st)
	}
}

func TestSeveralGamesJoinOneTree(t *testing.T) {
	a := newAPI(t)
	text := "[Orientation \"white\"]\n\n1. e4 e5 2. Nf3 *\n\n[Orientation \"white\"]\n\n1. e4 c5 2. Nf3 *\n\n[Orientation \"white\"]\n\n1. d4 d5 *\n"
	st := a.mustCall("POST", "/api/repertoires/import", importBody("Главы", "", text), 201)["import"].(map[string]any)
	// e4 e5 Nf3 c5 Nf3 d4 d5
	if st["games"] != 3.0 || st["added"] != 7.0 {
		t.Errorf("%v", st)
	}
}

func TestExportRoundTrip(t *testing.T) {
	a := newAPI(t)
	out := a.mustCall("POST", "/api/repertoires/import", importBody("Мой «репозиторий»: тест?", "", italianPGN), 201)
	rep := out["repertoire"].(map[string]any)
	base := fmt.Sprintf("/api/repertoires/%d", id(rep["id"]))

	req := httptest.NewRequest("GET", base+"/export", nil)
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/x-chess-pgn") {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	disp := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disp, "attachment;") || strings.ContainsAny(strings.TrimPrefix(disp, "attachment;"), "«»:?") && !strings.Contains(disp, "%") {
		t.Errorf("disposition %q", disp)
	}
	text := rec.Body.String()
	for _, want := range []string{`[Orientation "white"]`, `[Event "Мой «репозиторий»: тест?"]`, "1. e4 e5 (1... c5 2. Nf3) 2. Nf3 Nc6 (2... d6) 3. Bc4"} {
		if !strings.Contains(strings.Join(strings.Fields(text), " "), strings.Join(strings.Fields(want), " ")) {
			t.Errorf("export lacks %q:\n%s", want, text)
		}
	}

	// the export imports back into the same tree, colour and name included
	back := a.mustCall("POST", "/api/repertoires/import", importBody("", "", text), 201)
	br := back["repertoire"].(map[string]any)
	if br["color"] != "white" || br["name"] != "Мой «репозиторий»: тест?" || back["import"].(map[string]any)["added"] != 8.0 {
		t.Errorf("round trip: %v", back)
	}
}

func TestExportEmptyRepertoire(t *testing.T) {
	a := newAPI(t)
	rep := a.mustCall("POST", "/api/repertoires", `{"name":"Пусто","color":"black"}`, 201)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/repertoires/%d/export", id(rep["id"])), nil)
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `[Orientation "black"]`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

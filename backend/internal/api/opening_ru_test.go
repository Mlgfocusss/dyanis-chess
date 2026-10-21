package api

import (
	"encoding/json"
	"testing"

	"dyanis-chess/backend/internal/analysis"
	"dyanis-chess/backend/internal/eco"
)

// A report saved before opening names were translated has only the English
// name; reading it back must add the Russian one, in the report and on its plies.
func TestLocalizeOpeningsFillsOldReports(t *testing.T) {
	old := []byte(`{"opening":{"eco":"C65","name":"Ruy Lopez: Berlin Defense","plies":6},
		"moves":[{"opening":{"eco":"C60","name":"Ruy Lopez","plies":5}},{}]}`)
	var rep analysis.Report
	if err := json.Unmarshal(old, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Opening.NameRU != "" {
		t.Fatalf("old report already has a Russian name: %q", rep.Opening.NameRU)
	}
	localizeOpenings(&rep)
	if want := "Испанская партия (Рюи Лопес): берлинская защита"; rep.Opening.NameRU != want {
		t.Errorf("report opening = %q, want %q", rep.Opening.NameRU, want)
	}
	if got := rep.Moves[0].Opening.NameRU; got == "" || got == "Ruy Lopez" {
		t.Errorf("ply opening = %q", got)
	}
	if rep.Moves[1].Opening != nil {
		t.Error("a ply without an opening gained one")
	}
	// A name already translated is left alone.
	o := &eco.Opening{Name: "Ruy Lopez", NameRU: "своё"}
	rep.Opening = o
	localizeOpenings(&rep)
	if o.NameRU != "своё" {
		t.Errorf("existing NameRU overwritten: %q", o.NameRU)
	}
}

func TestSummarizeCarriesRussianOpening(t *testing.T) {
	rep := &analysis.Report{Opening: &eco.Opening{ECO: "B90", Name: "Sicilian Defense: Najdorf Variation", NameRU: eco.TranslateName("Sicilian Defense: Najdorf Variation")}}
	sm := summarize(rep)
	if sm.Opening != "Sicilian Defense: Najdorf Variation" || sm.OpeningRU != "Сицилианская защита: вариант Найдорфа" {
		t.Errorf("summary = %+v", sm)
	}
}

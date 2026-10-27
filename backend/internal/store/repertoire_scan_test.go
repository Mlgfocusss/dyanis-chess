package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Needs a real MySQL, see openTest.
func TestRepScans(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	anna, boris := newUserID(t, s, KindGuest), newUserID(t, s, KindGuest)
	rep, err := s.CreateRepertoire(ctx, anna.ID, "Сицилианка", "black")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateRepertoire(ctx, anna.ID, "Пустой", "white")

	// 1.e4 (opponent) c5 (mine) 2.Nf3 (opponent, no answer) and 1.d4 (opponent, no answer)
	add := func(parent int64, uci string, ply int, mine bool) RepMove {
		t.Helper()
		m, _, err := s.AddRepMove(ctx, RepMove{RepertoireID: rep.ID, ParentID: parent, UCI: uci, SAN: uci, FEN: "fen " + uci, Ply: ply, Mine: mine})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	e4 := add(0, "e2e4", 1, false)
	c5 := add(e4.ID, "c7c5", 2, true)
	nf3 := add(c5.ID, "g1f3", 3, false)
	d4 := add(0, "d2d4", 1, false)

	// no search yet: the overview still counts the lines without an answer
	ov, err := s.RepOverviews(ctx, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o := ov[rep.ID]; o.Open != 2 || o.MaxMove != d4.ID || o.Scan != nil {
		t.Errorf("overview before a search: %+v", o)
	}
	if _, ok := ov[other.ID]; ok {
		t.Errorf("an empty repertoire has no overview row: %+v", ov[other.ID])
	}
	if _, err := s.RepScan(ctx, anna.ID, rep.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a search that was never made: %v", err)
	}

	scan := RepScan{RepertoireID: rep.ID, ScannedAt: time.Now().UTC().Truncate(time.Millisecond), Moves: 4, MaxMoveID: d4.ID,
		Missing: 3, Answer: 2, Coverage: 0.25, Partial: true, Source: "mixed", Result: `{"gaps":[{"san":"Nc3"}],"текст":"Лошадь"}`}
	if err := s.SaveRepScan(ctx, boris.ID, scan); !errors.Is(err, ErrNotFound) {
		t.Errorf("saving into somebody else's repertoire: %v", err)
	}
	if err := s.SaveRepScan(ctx, anna.ID, scan); err != nil {
		t.Fatal(err)
	}
	got, err := s.RepScan(ctx, anna.ID, rep.ID)
	if err != nil || got.Result != scan.Result || got.Coverage != 0.25 || !got.Partial || got.Source != "mixed" || got.Moves != 4 ||
		got.Missing != 3 || got.Answer != 2 || got.MaxMoveID != d4.ID || !got.ScannedAt.Equal(scan.ScannedAt) {
		t.Errorf("saved search: %+v (%v)", got, err)
	}
	if _, err := s.RepScan(ctx, boris.ID, rep.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("somebody else reads the search: %v", err)
	}

	// a new search replaces the old one (one row per repertoire)
	scan.Missing, scan.Coverage, scan.Partial = 1, 0.9, false
	if err := s.SaveRepScan(ctx, anna.ID, scan); err != nil {
		t.Fatal(err)
	}
	ov, _ = s.RepOverviews(ctx, anna.ID)
	if o := ov[rep.ID]; o.Scan == nil || o.Scan.Missing != 1 || o.Scan.Coverage != 0.9 || o.Scan.Partial || o.Scan.Result != "" || o.Open != 2 {
		t.Errorf("overview after a search: %+v scan=%+v", o, o.Scan)
	}
	if other, _ := s.RepOverviews(ctx, boris.ID); len(other) != 0 {
		t.Errorf("boris sees %v", other)
	}

	// answering both lines empties the first kind of hole; the tree's size changes the signature
	add(nf3.ID, "b1c3", 4, true)
	add(d4.ID, "g8f6", 2, true)
	ov, _ = s.RepOverviews(ctx, anna.ID)
	if o := ov[rep.ID]; o.Open != 0 || o.MaxMove == d4.ID {
		t.Errorf("overview after answering: %+v", o)
	}

	// the search goes with the repertoire
	if err := s.DeleteRepertoire(ctx, anna.ID, rep.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM rep_scans WHERE repertoire_id = ?", rep.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("the search outlived the repertoire: %d rows (%v)", n, err)
	}
}

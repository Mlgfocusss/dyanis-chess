package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLoadMigrations(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) == 0 || migs[0].version != 1 {
		t.Fatalf("migrations: %+v", migs)
	}
	for i := 1; i < len(migs); i++ {
		if migs[i].version != migs[i-1].version+1 {
			t.Errorf("gap between migration %d and %d", migs[i-1].version, migs[i].version)
		}
	}
}

// The tests below need a real MySQL: point DYANIS_TEST_DSN at one, e.g.
//
//	docker compose up -d db
//	DYANIS_TEST_DSN='dyanis:PASSWORD@tcp(127.0.0.1:3307)/dyanis' go test ./internal/store
func openTest(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("DYANIS_TEST_DSN")
	if dsn == "" {
		t.Skip("DYANIS_TEST_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.migrate(ctx); err != nil { // second run: nothing to do
		t.Fatal(err)
	}
	var n, max int
	if err := s.db.QueryRow("SELECT COUNT(*), MAX(version) FROM schema_migrations").Scan(&n, &max); err != nil {
		t.Fatal(err)
	}
	migs, _ := loadMigrations()
	if n != len(migs) || max != migs[len(migs)-1].version {
		t.Errorf("recorded %d migrations up to %d, have %d", n, max, len(migs))
	}
}

func TestAnalysisRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	id := fmt.Sprintf("t%015d", time.Now().UnixNano()%1_000_000_000_000_000) // 16 chars, like real ids
	t.Cleanup(func() { s.db.Exec("DELETE FROM analyses WHERE id = ?", id) })

	if _, err := s.LoadAnalysis(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing row: %v", err)
	}
	game, _ := json.Marshal(map[string]string{"white": "Анна", "black": "Борис 🙂"})
	report := []byte(`{"accuracy":[83.1,94.3],"moves":[]}`)
	created := time.Now().UTC().Truncate(time.Millisecond)
	a := &Analysis{ID: id, Status: "done", Depth: 14, Game: game, Report: report,
		CreatedAt: created, FinishedAt: created.Add(2 * time.Second)}
	if err := s.SaveAnalysis(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadAnalysis(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]string
	if err := json.Unmarshal(got.Game, &g); err != nil || g["white"] != "Анна" || g["black"] != "Борис 🙂" {
		t.Errorf("game round trip: %s (%v)", got.Game, err)
	}
	var r struct{ Accuracy [2]float64 }
	if err := json.Unmarshal(got.Report, &r); err != nil || r.Accuracy[1] != 94.3 {
		t.Errorf("report round trip: %s (%v)", got.Report, err)
	}
	if got.Status != "done" || got.Depth != 14 || got.Error != "" || !got.CreatedAt.Equal(created) {
		t.Errorf("fields: %+v", got)
	}

	// An error result has no report; saving again replaces the row.
	a.Status, a.Report, a.Error = "error", nil, "boom"
	if err := s.SaveAnalysis(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadAnalysis(ctx, id)
	if err != nil || got.Status != "error" || got.Report != nil || got.Error != "boom" {
		t.Errorf("error row: %+v (%v)", got, err)
	}
}

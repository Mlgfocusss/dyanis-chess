// Command puzzleimport loads a subset of the Lichess puzzle database into
// MySQL, for the puzzle trainer.
//
// Get the file (CC0, about 250 MB, 4 million puzzles) from
// https://database.lichess.org/#puzzles and keep it outside git (data/ is
// ignored):
//
//	go run ./backend/cmd/puzzleimport -file data/puzzles/lichess_db_puzzle.csv.zst \
//	    -dsn "dyanis:PASSWORD@tcp(127.0.0.1:3317)/dyanis"
//
// It keeps the most popular puzzles of every 100-point rating bucket (the
// default 100000 in all), so easy and hard puzzles are both plentiful, and
// drops any whose solution is not a legal line. Running it again adds only
// the puzzles that are not there yet. -dry-run only reports what it would load.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"dyanis-chess/backend/internal/store"
)

func main() {
	file := flag.String("file", "", "Lichess puzzle CSV, plain or .zst (required)")
	dsn := flag.String("dsn", os.Getenv("DYANIS_DB_DSN"), "MySQL DSN (user:pass@tcp(host:3306)/db)")
	total := flag.Int("total", 100000, "about how many puzzles to keep in all")
	bucket := flag.Int("bucket", 100, "width of a rating bucket")
	maxDev := flag.Int("maxdev", 90, "drop puzzles whose rating deviation is above this")
	minPop := flag.Int("minpop", 85, "drop puzzles whose popularity (-100..100) is below this")
	minPlays := flag.Int("minplays", 200, "drop puzzles played fewer times than this")
	dry := flag.Bool("dry-run", false, "read the file and report, without touching the database")
	flag.Parse()
	if *file == "" || (*dsn == "" && !*dry) {
		flag.Usage()
		os.Exit(2)
	}

	f, err := os.Open(*file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(*file, ".zst") {
		zr, err := zstd.NewReader(f)
		if err != nil {
			log.Fatal(err)
		}
		defer zr.Close()
		r = zr
	}

	// The rating span of Lichess puzzles is about 400..3400.
	buckets := max(1, (3400-400) / *bucket)
	c := criteria{maxDeviation: *maxDev, minPopularity: *minPop, minPlays: *minPlays, bucket: *bucket, perBucket: *total / buckets}
	start := time.Now()
	ps, st, err := pick(r, c)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("read %d rows in %s: %d too weak or unsettled, %d malformed, %d with an illegal line; keeping %d",
		st.rows, time.Since(start).Round(time.Second), st.filtered, st.malformed, st.badLine, len(ps))
	report(ps, *bucket)
	if *dry {
		return
	}

	ctx := context.Background()
	db, err := store.Open(ctx, *dsn, 30*time.Second)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()
	added := 0
	for i := 0; i < len(ps); i += 10000 {
		n, err := db.ImportPuzzles(ctx, ps[i:min(i+10000, len(ps))])
		if err != nil {
			log.Fatalf("import: %v", err)
		}
		added += n
		log.Printf("%d / %d", min(i+10000, len(ps)), len(ps))
	}
	have, _ := db.CountPuzzles(ctx)
	log.Printf("added %d new puzzles; the database now has %d", added, have)
}

// report prints how many puzzles each rating range got.
func report(ps []store.Puzzle, width int) {
	count := map[int]int{}
	for _, p := range ps {
		count[p.Rating/width]++
	}
	var b strings.Builder
	for k := 0; k < 40; k++ {
		if count[k] > 0 {
			fmt.Fprintf(&b, "  %4d-%4d: %d\n", k*width, (k+1)*width-1, count[k])
		}
	}
	log.Printf("by rating:\n%s", b.String())
}

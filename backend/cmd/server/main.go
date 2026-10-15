// Command server runs the web back end: the game-review API of
// backend/internal/api, and optionally the built front end (web/dist).
//
//	server -evalfile net-kingf-m4.bin [-dsn "user:pass@tcp(localhost:3306)/dyanis"]
//	server -evalfile net-kingf-m4.bin -static web/dist -addr :8080
//
// In development the front end runs under Vite (npm run dev in web/),
// which proxies /api to this server; -static is for a single-process run
// without Node. In the Docker setup nginx serves the static files.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"dyanis-chess/backend/internal/api"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/engine"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	evalFile := flag.String("evalfile", "", "NNUE weights file; empty = hand-written eval")
	depth := flag.Int("depth", 14, "default search depth per position")
	maxDepth := flag.Int("maxdepth", 20, "largest depth a request may ask for")
	static := flag.String("static", "", "directory with the built front end to serve at /")
	dsn := flag.String("dsn", os.Getenv("DYANIS_DB_DSN"), "MySQL DSN (user:pass@tcp(host:3306)/db); empty = keep results in memory only")
	flag.Parse()

	if *evalFile != "" {
		if err := engine.LoadNetwork(*evalFile); err != nil {
			log.Fatalf("evalfile: %v", err)
		}
		log.Printf("network %s loaded", *evalFile)
	} else {
		log.Printf("no -evalfile: using the hand-written evaluation")
	}

	cfg := api.Config{DefaultDepth: *depth, MaxDepth: *maxDepth}
	if *dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		db, err := store.Open(ctx, *dsn, 60*time.Second)
		cancel()
		if err != nil {
			log.Fatalf("database: %v", err)
		}
		defer db.Close()
		cfg.Store = db
		log.Printf("database connected, schema up to date")
	} else {
		log.Printf("no -dsn: results are kept in memory only")
	}
	srv := api.New(cfg)
	defer srv.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	if *static != "" {
		mux.Handle("/", spa(*static))
	}
	hs := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(ctx)
	}()

	log.Printf("listening on http://%s", *addr)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// spa serves dir, falling back to index.html for paths that are not files
// so that client-side routes survive a page reload.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if st, err := os.Stat(p); err != nil || st.IsDir() && r.URL.Path != "/" {
			if filepath.Ext(r.URL.Path) == "" {
				http.ServeFile(w, r, filepath.Join(dir, "index.html"))
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

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
	"dyanis-chess/backend/internal/auth"
	"dyanis-chess/backend/internal/explorer"
	"dyanis-chess/backend/internal/match"
	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/puzzles"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/repertoire"
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
	publicURL := flag.String("public-url", envOr("PUBLIC_URL", "http://localhost:8080"), "address people open the site at; Google redirects back to it")
	googleID := flag.String("google-client-id", os.Getenv("GOOGLE_CLIENT_ID"), "Google OAuth client id (empty = no Google sign-in)")
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
	var accounts *auth.Service
	var games *play.Hub
	var rated *ratings.Service
	var matcher *match.Service
	var puzzleAPI *puzzles.Service
	var repertoireAPI *repertoire.Service
	var explorerAPI *explorer.Service
	if *dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		db, err := store.Open(ctx, *dsn, 60*time.Second)
		cancel()
		if err != nil {
			log.Fatalf("database: %v", err)
		}
		defer db.Close()
		cfg.Store = db
		if err := db.PurgeExpiredSessions(context.Background()); err != nil {
			log.Printf("purge sessions: %v", err)
		}
		accounts = auth.New(auth.Config{
			Store:        db,
			CookieSecure: strings.HasPrefix(*publicURL, "https://"),
			Google: auth.Google{
				ClientID:     *googleID,
				ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"), // not a flag: flags show up in process lists and -h
				RedirectURL:  strings.TrimRight(*publicURL, "/") + "/api/auth/google/callback",
			},
		})
		cfg.UserID = accounts.UserID
		rated = ratings.New(db, nil)
		games = play.New(play.Config{Store: db, User: accounts.UserFromRequest, Ratings: rated})
		defer games.Close()
		puzzleAPI = puzzles.New(puzzles.Config{Store: db, UserID: accounts.UserID, Ratings: rated})
		// Matchmaking brings two people together for a game with a clock or a puzzle duel.
		matcher = match.New(match.Config{
			User:   accounts.UserFromRequest,
			Rating: rated.Get,
			Chess: func(ctx context.Context, a, b match.Seeker, req match.Request) (string, error) {
				return games.NewMatched(ctx, play.Player{ID: a.ID, Name: a.Name, Avatar: a.Avatar}, play.Player{ID: b.ID, Name: b.Name, Avatar: b.Avatar},
					play.TimeControl{BaseMs: req.BaseMs, IncMs: req.IncMs}, req.Rated)
			},
			Duel: func(ctx context.Context, a, b match.Seeker, _ match.Request) (string, error) {
				return puzzleAPI.NewDuel(ctx, puzzles.DuelPlayer{ID: a.ID, Name: a.Name, Avatar: a.Avatar, Rating: a.Rating, Provisional: a.Provisional},
					puzzles.DuelPlayer{ID: b.ID, Name: b.Name, Avatar: b.Avatar, Rating: b.Rating, Provisional: b.Provisional})
			},
		})
		defer matcher.Close()
		repertoireAPI = repertoire.New(repertoire.Config{Store: db, UserID: accounts.UserID})
		// Statistics of the Lichess opening explorer for the repository builder. The
		// explorer wants a token; the environment, not a flag, so that it stays out of process lists.
		explorerAPI = explorer.New(explorer.Config{Token: os.Getenv("LICHESS_TOKEN"), UserID: accounts.UserID})
		if os.Getenv("LICHESS_TOKEN") == "" {
			log.Printf("no LICHESS_TOKEN: the repository builder shows no Lichess statistics (see DEVELOPERS.md)")
		}
		if n, err := db.CountPuzzles(context.Background()); err == nil && n == 0 {
			log.Printf("no puzzles yet: load them with backend/cmd/puzzleimport (see DEVELOPERS.md)")
		}
		log.Printf("database connected, schema up to date")
	} else {
		log.Printf("no -dsn: results are kept in memory only, and there are no accounts")
	}
	srv := api.New(cfg)
	defer srv.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	if accounts != nil {
		mux.Handle("/api/me", accounts.Handler())
		mux.Handle("/api/me/", accounts.Handler())
		mux.Handle("/api/auth/", accounts.Handler())
		mux.Handle("/api/games", games.Handler())
		mux.Handle("/api/games/", games.Handler())
		mux.Handle("/api/bots", games.Handler())
		mux.Handle("/api/puzzles/", puzzleAPI.Handler())
		mux.Handle("/api/duels/", puzzleAPI.Handler())
		mux.Handle("/api/match/", matcher.Handler())
		mux.Handle("/api/ratings", rated.Handler(accounts.UserID))
		mux.Handle("/api/repertoires", repertoireAPI.Handler())
		mux.Handle("/api/repertoires/", repertoireAPI.Handler())
		mux.Handle("/api/explorer", explorerAPI.Handler())
	}
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

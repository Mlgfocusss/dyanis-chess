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

	"dyanis-chess/backend/internal/achievements"
	"dyanis-chess/backend/internal/api"
	"dyanis-chess/backend/internal/auth"
	"dyanis-chess/backend/internal/coach"
	"dyanis-chess/backend/internal/community"
	"dyanis-chess/backend/internal/countries"
	"dyanis-chess/backend/internal/explorer"
	"dyanis-chess/backend/internal/forum"
	"dyanis-chess/backend/internal/lessons"
	"dyanis-chess/backend/internal/match"
	"dyanis-chess/backend/internal/play"
	"dyanis-chess/backend/internal/profiles"
	"dyanis-chess/backend/internal/puzzles"
	"dyanis-chess/backend/internal/ratings"
	"dyanis-chess/backend/internal/repertoire"
	"dyanis-chess/backend/internal/stats"
	"dyanis-chess/backend/internal/store"
	"dyanis-chess/backend/internal/trainer"
	"dyanis-chess/backend/internal/vplay"
	"dyanis-chess/engine"
)

// splitList reads a comma-separated list ("a, b,c"); empty text gives nil.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

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
	var variantGames *vplay.Hub
	var rated *ratings.Service
	var profilesAPI *profiles.Service
	var matcher *match.Service
	var puzzleAPI *puzzles.Service
	var statsAPI *stats.Service
	var lessonsAPI *lessons.Service
	var achievementsAPI *achievements.Service
	var trainerAPI *trainer.Service
	var repertoireAPI *repertoire.Service
	var explorerAPI *explorer.Service
	var communityAPI *community.Service
	var forumAPI *forum.Service
	var coachNotes coach.Store // where explanations are kept; nil without a database
	if *dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		db, err := store.Open(ctx, *dsn, 60*time.Second)
		cancel()
		if err != nil {
			log.Fatalf("database: %v", err)
		}
		defer db.Close()
		cfg.Store = db
		coachNotes = db
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
		profilesAPI = profiles.New(db, rated, nil)
		// Friends, messages, challenges and the vacations of correspondence games. A challenge starts
		// its game in the hub made just below, which in turn asks the community for the settings
		// that the deadlines of correspondence games depend on.
		communityAPI = community.New(community.Config{
			Store: db,
			User:  accounts.UserFromRequest,
			Start: func(ctx context.Context, white, black play.Player, tc play.TimeControl, rated bool) (string, error) {
				return games.NewChallenged(ctx, white, black, tc, rated)
			},
			Rating: rated.Get,
			Correspondence: func(ctx context.Context, userID string) ([]play.CorrGame, error) {
				return games.Correspondence(ctx, userID)
			},
		})
		games = play.New(play.Config{Store: db, User: accounts.UserFromRequest, Ratings: rated, Prefs: communityAPI.PlayPrefs})
		defer games.Close()
		// Correspondence games run on days: bring the unfinished ones back so that their deadlines count.
		rctx, rcancel := context.WithTimeout(context.Background(), 60*time.Second)
		games.Resume(rctx)
		rcancel()
		sweepStop := make(chan struct{})
		defer close(sweepStop)
		go func() {
			t := time.NewTicker(10 * time.Minute)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					communityAPI.Sweep()
				case <-sweepStop:
					return
				}
			}
		}()
		forumAPI = forum.New(forum.Config{Store: db, User: accounts.UserFromRequest, Admins: splitList(os.Getenv("FORUM_ADMINS"))})
		// Games of the chess variants ("Другие шахматы"): people only, no rating, no engine.
		variantGames = vplay.New(vplay.Config{Store: db, User: accounts.UserFromRequest})
		defer variantGames.Close()
		puzzleAPI = puzzles.New(puzzles.Config{Store: db, UserID: accounts.UserID, Ratings: rated})
		statsAPI = stats.New(stats.Config{Store: db, Ratings: rated, UserID: accounts.UserID})
		// The lessons are written by hand and checked at start-up: a broken lesson stops the server.
		lessonLibrary, err := lessons.Load()
		if err != nil {
			log.Fatalf("lessons: %v", err)
		}
		lessonsAPI = lessons.New(lessons.Config{Store: db, UserID: accounts.UserID, Library: lessonLibrary})
		// Achievements are computed on reading from the other tables; the lessons give their sections.
		var lessonSections [][]string
		for _, sec := range lessonLibrary.Sections {
			ids := make([]string, 0, len(sec.Lessons))
			for _, l := range sec.Lessons {
				ids = append(ids, l.ID)
			}
			lessonSections = append(lessonSections, ids)
		}
		achievementsAPI = achievements.New(achievements.Config{Store: db, UserID: accounts.UserID, Sections: lessonSections})
		trainerAPI = trainer.New(trainer.Config{Store: db, UserID: accounts.UserID})
		// Matchmaking brings two people together for a game with a clock or a puzzle duel.
		matcher = match.New(match.Config{
			User:   accounts.UserFromRequest,
			Rating: rated.Get,
			Avoid:  db.BlockedWith, // people who blocked each other are never paired
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
		// Statistics of the Lichess opening explorer for the repository builder. The
		// explorer wants a token; the environment, not a flag, so that it stays out of process lists.
		// LICHESS_EXPLORER_URL replaces the address of the explorer (a mirror, or a stand-in for tests).
		explorerAPI = explorer.New(explorer.Config{Token: os.Getenv("LICHESS_TOKEN"), BaseURL: os.Getenv("LICHESS_EXPLORER_URL"), UserID: accounts.UserID})
		// the repository's search for holes asks the same explorer (and its cache)
		repertoireAPI = repertoire.New(repertoire.Config{Store: db, UserID: accounts.UserID, Stats: explorerAPI})
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
	// The AI coach explains moves of a review. The key and the model come from the environment
	// (not flags: those show up in process lists); without a key the coach stays off.
	cfg.Coach = coach.New(coach.Config{
		APIKey: os.Getenv("OPENROUTER_API_KEY"),
		Models: splitList(os.Getenv("OPENROUTER_MODELS")),
		Site:   *publicURL,
		Store:  coachNotes,
	})
	if cfg.Coach.Enabled() {
		log.Printf("AI coach on (OpenRouter)")
	} else {
		log.Printf("no OPENROUTER_API_KEY: the AI coach is off (see DEVELOPERS.md)")
	}
	if variantGames != nil {
		// The review of a finished Chess960 game takes its actions from the variants hub.
		cfg.VariantGame = func(ctx context.Context, id string) (*api.VariantSource, error) {
			g, err := variantGames.ReviewGame(ctx, id)
			if errors.Is(err, vplay.ErrNoGame) {
				return nil, api.ErrNoVariantGame
			}
			if err != nil {
				return nil, err
			}
			return &api.VariantSource{Variant: g.Variant, Options: g.Options, Actions: g.Actions, Status: g.Status, Result: g.Result, White: g.White, Black: g.Black}, nil
		}
	}
	srv := api.New(cfg)
	defer srv.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	mux.Handle("/api/countries", countries.Handler()) // public and needs no database
	if accounts != nil {
		mux.Handle("/api/me", accounts.Handler())
		mux.Handle("/api/me/", accounts.Handler())
		mux.Handle("/api/auth/", accounts.Handler())
		mux.Handle("/api/games", games.Handler())
		mux.Handle("/api/games/", games.Handler())
		mux.Handle("/api/bots", games.Handler())
		mux.Handle("/api/variants", variantGames.Handler())
		mux.Handle("/api/variants/", variantGames.Handler())
		mux.Handle("/api/puzzles/", puzzleAPI.Handler())
		mux.Handle("/api/duels/", puzzleAPI.Handler())
		mux.Handle("/api/match/", matcher.Handler())
		mux.Handle("/api/ratings", rated.Handler(accounts.UserID))
		mux.Handle("/api/stats", statsAPI.Handler())
		mux.Handle("/api/achievements", achievementsAPI.Handler())
		mux.Handle("/api/achievements/", achievementsAPI.Handler())
		mux.Handle("/api/users/{id}/achievements", achievementsAPI.Handler()) // more specific than /api/users/ below
		mux.Handle("/api/lessons", lessonsAPI.Handler())
		mux.Handle("/api/lessons/", lessonsAPI.Handler())
		mux.Handle("/api/trainer", trainerAPI.Handler())
		mux.Handle("/api/trainer/", trainerAPI.Handler())
		mux.Handle("/api/leaderboard", profilesAPI.Handler())
		mux.Handle("/api/users/", profilesAPI.Handler())
		mux.Handle("/api/repertoires", repertoireAPI.Handler())
		mux.Handle("/api/repertoires/", repertoireAPI.Handler())
		mux.Handle("/api/explorer", explorerAPI.Handler())
		mux.Handle("/api/community/", communityAPI.Handler())
		mux.Handle("/api/forum/", forumAPI.Handler())
		mux.Handle("/api/reports", forumAPI.Handler()) // complaints: made by anybody signed in, read by the moderators
		mux.Handle("/api/reports/", forumAPI.Handler())
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

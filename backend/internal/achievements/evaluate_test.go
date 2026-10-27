package achievements

import (
	"strconv"
	"testing"

	"dyanis-chess/backend/internal/store"
)

func find(t *testing.T, ps []Progress, id string) Progress {
	t.Helper()
	for _, p := range ps {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("no badge %s", id)
	return Progress{}
}

func TestNobodyEarnsAnything(t *testing.T) {
	for _, p := range Evaluate(Facts{}) {
		if p.Done {
			t.Errorf("%s is done for a person with nothing", p.ID)
		}
		if p.Current != 0 {
			t.Errorf("%s: progress %d", p.ID, p.Current)
		}
	}
}

// The edge of every counted badge: one short of the target is not enough, the target is.
func TestBoundaries(t *testing.T) {
	set := map[Metric]func(*Facts, int){
		MGames:        func(f *Facts, n int) { f.Games = n },
		MHumanWins:    func(f *Facts, n int) { f.HumanWins = n },
		MRatedWins:    func(f *Facts, n int) { f.RatedWins = n },
		MMateWins:     func(f *Facts, n int) { f.MateWins = n },
		MTimeWins:     func(f *Facts, n int) { f.TimeWins = n },
		MDraws:        func(f *Facts, n int) { f.Draws = n },
		MLongGames:    func(f *Facts, n int) { f.LongGames = n },
		MBotsBeaten:   func(f *Facts, n int) { f.BotsBeaten = n },
		MBeatMax:      func(f *Facts, n int) { f.MaxBeaten = n > 0 },
		MRating:       func(f *Facts, n int) { f.RatingMax = n },
		MDuels:        func(f *Facts, n int) { f.DuelGames = n },
		MDuelWins:     func(f *Facts, n int) { f.DuelWins = n },
		MPuzzles:      func(f *Facts, n int) { f.PuzzlesSolved = n },
		MPuzzleRating: func(f *Facts, n int) { f.PuzzleRating = n },
		MPointsStreak: func(f *Facts, n int) { f.BestPointsStreak = n },
		MRush:         func(f *Facts, n int) { f.RushBest = n },
		MReviews:      func(f *Facts, n int) { f.Reviews = n },
		MAccuracy:     func(f *Facts, n int) { f.AccuracyHigh = n > 0 },
		MBrilliant:    func(f *Facts, n int) { f.Brilliant = n > 0 },
		MRepertoires:  func(f *Facts, n int) { f.Repertoires = n },
		MRepMoves:     func(f *Facts, n int) { f.RepMoves = n },
		MLearnedMoves: func(f *Facts, n int) { f.LearnedMoves = n },
		MFriends:      func(f *Facts, n int) { f.Friends = n },
		MForumPosts:   func(f *Facts, n int) { f.ForumPosts = n },
		MCorrGames:    func(f *Facts, n int) { f.CorrGames = n },
		MStreakDays:   func(f *Facts, n int) { f.StreakDays = n },
	}
	for _, d := range Catalog {
		put, ok := set[d.Metric]
		if !ok {
			continue // the lesson badges have their own test
		}
		var below, at Facts
		put(&below, d.Target-1)
		put(&at, d.Target)
		if p := find(t, Evaluate(below), d.ID); p.Done || p.Current != d.Target-1 {
			t.Errorf("%s: %d of %d must not be enough (%+v)", d.ID, d.Target-1, d.Target, p)
		}
		if p := find(t, Evaluate(at), d.ID); !p.Done || p.Current != d.Target {
			t.Errorf("%s: %d of %d must be enough (%+v)", d.ID, d.Target, d.Target, p)
		}
		// far above: done, and the progress is held at the target
		var over Facts
		put(&over, d.Target*3)
		if p := find(t, Evaluate(over), d.ID); !p.Done || p.Current != d.Target {
			t.Errorf("%s: far over the target: %+v", d.ID, p)
		}
	}
}

func TestOneNumberOpensSeveralBadges(t *testing.T) {
	ps := Evaluate(Facts{AchievementFacts: store.AchievementFacts{Games: 120, BotsBeaten: 5, RatingMax: 1650}})
	want := map[string]bool{
		"first_game": true, "games_10": true, "games_100": true, "games_500": false,
		"first_bot_win": true, "bots_3": true, "bots_5": true, "bots_10": false,
		"rating_1400": true, "rating_1600": true, "rating_1800": false,
	}
	for id, done := range want {
		if p := find(t, ps, id); p.Done != done {
			t.Errorf("%s: done = %v", id, p.Done)
		}
	}
	if p := find(t, ps, "games_500"); p.Current != 120 || p.Target != 500 || !p.ShowsProgress() {
		t.Errorf("games_500 progress: %+v", p)
	}
	if find(t, ps, "first_game").ShowsProgress() {
		t.Error("a yes-or-no badge has no progress bar")
	}
}

func TestLessonBadges(t *testing.T) {
	sections := [][]string{{"a1", "a2"}, {"b1", "b2", "b3"}}
	mk := func(done ...string) []Progress {
		return Evaluate(Facts{AchievementFacts: store.AchievementFacts{LessonsDone: done}, Sections: sections})
	}
	if ps := mk(); find(t, ps, "first_lesson").Done || find(t, ps, "lesson_section").Done || find(t, ps, "all_lessons").Done {
		t.Error("nothing is done yet")
	}
	ps := mk("a1")
	if !find(t, ps, "first_lesson").Done || find(t, ps, "lesson_section").Done {
		t.Error("one lesson of a section is a first lesson, not a section")
	}
	ps = mk("a1", "a2")
	if !find(t, ps, "lesson_section").Done || find(t, ps, "all_lessons").Done {
		t.Error("a whole section")
	}
	if p := find(t, ps, "all_lessons"); p.Target != 5 || p.Current != 2 || !p.ShowsProgress() {
		t.Errorf("the whole course counts its lessons: %+v", p)
	}
	ps = mk("a1", "a2", "b1", "b2", "b3")
	if !find(t, ps, "all_lessons").Done {
		t.Error("all lessons")
	}
	// a lesson that was removed from the course does not count towards "all", and without a course nothing is done
	if find(t, mk("a1", "a2", "b1", "b2", "gone", "gone2"), "all_lessons").Done {
		t.Error("removed lessons must not fill the course")
	}
	ps = Evaluate(Facts{AchievementFacts: store.AchievementFacts{LessonsDone: []string{"x"}}})
	if find(t, ps, "all_lessons").Done || find(t, ps, "lesson_section").Done {
		t.Error("without a course nothing is whole")
	}
}

func TestOwnSideOfReviews(t *testing.T) {
	game := []byte(`{"white":"Гость 1","black":"Миша"}`)
	sum := func(w, b float64) []byte { return []byte(`{"accuracy":[` + ftoa(w) + `,` + ftoa(b) + `],"plies":40}`) }
	rows := []store.ReviewBrief{{Game: game, Summary: sum(50, 95)}}
	if ownAccuracyHigh("Гость 1", rows) {
		t.Error("the 95 is the bot's, not the person's")
	}
	if !ownAccuracyHigh("гость 1 ", []store.ReviewBrief{{Game: game, Summary: sum(90, 40)}}) {
		t.Error("90 is the border and the name is compared loosely")
	}
	if ownAccuracyHigh("Гость 1", []store.ReviewBrief{{Game: game, Summary: sum(89.9, 40)}}) {
		t.Error("89.9 is not enough")
	}
	if ownAccuracyHigh("Кто-то", []store.ReviewBrief{{Game: game, Summary: sum(99, 99)}}) {
		t.Error("a game in which the person cannot be found proves nothing")
	}
	if ownAccuracyHigh("Гость 1", []store.ReviewBrief{{Game: []byte(`oops`), Summary: sum(99, 99)}}) {
		t.Error("an unreadable row is skipped")
	}

	b := []store.BrilliantReview{{Game: game, Moves: []int{3, 5}}} // half-moves 4 and 6: Black's
	if ownBrilliant("Гость 1", b) {
		t.Error("the brilliant moves are Black's")
	}
	if !ownBrilliant("Миша", b) {
		t.Error("Black's own brilliant moves")
	}
	if !ownBrilliant("Гость 1", []store.BrilliantReview{{Game: game, Moves: []int{3, 8}}}) {
		t.Error("index 8 is White's fifth move")
	}
	if ownBrilliant("Никто", b) {
		t.Error("unknown person")
	}
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

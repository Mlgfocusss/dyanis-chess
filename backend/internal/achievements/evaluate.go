package achievements

import "dyanis-chess/backend/internal/store"

// Facts are the aggregates of one person that the badges are decided by: the counters of the store
// plus the few numbers that the service works out from other sources.
type Facts struct {
	store.AchievementFacts
	// AccuracyHigh and Brilliant say that a game of the person has an accuracy of AccuracyHigh or more /
	// a brilliant move of their own.
	AccuracyHigh bool
	Brilliant    bool
	// StreakDays is the best series of days with puzzles.
	StreakDays int
	// Sections are the lessons of each section of the course (ids); the total is all of them.
	Sections [][]string
}

// Value is the number the metric stands for.
func (f Facts) Value(m Metric) int {
	switch m {
	case MGames:
		return f.Games
	case MHumanWins:
		return f.HumanWins
	case MRatedWins:
		return f.RatedWins
	case MMateWins:
		return f.MateWins
	case MTimeWins:
		return f.TimeWins
	case MDraws:
		return f.Draws
	case MLongGames:
		return f.LongGames
	case MBotsBeaten:
		return f.BotsBeaten
	case MBeatMax:
		return b2i(f.MaxBeaten)
	case MRating:
		return f.RatingMax
	case MDuels:
		return f.DuelGames
	case MDuelWins:
		return f.DuelWins
	case MPuzzles:
		return f.PuzzlesSolved
	case MPuzzleRating:
		return f.PuzzleRating
	case MPointsStreak:
		return f.BestPointsStreak
	case MRush:
		return f.RushBest
	case MReviews:
		return f.Reviews
	case MAccuracy:
		return b2i(f.AccuracyHigh)
	case MBrilliant:
		return b2i(f.Brilliant)
	case MRepertoires:
		return f.Repertoires
	case MRepMoves:
		return f.RepMoves
	case MLearnedMoves:
		return f.LearnedMoves
	case MLessons:
		return len(f.LessonsDone)
	case MSections:
		return f.sectionsDone()
	case MAllLessons:
		return f.knownLessonsDone()
	case MFriends:
		return f.Friends
	case MForumPosts:
		return f.ForumPosts
	case MCorrGames:
		return f.CorrGames
	case MStreakDays:
		return f.StreakDays
	}
	return 0
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (f Facts) doneSet() map[string]bool {
	m := make(map[string]bool, len(f.LessonsDone))
	for _, id := range f.LessonsDone {
		m[id] = true
	}
	return m
}

// sectionsDone counts the sections of which every lesson is finished.
func (f Facts) sectionsDone() int {
	done := f.doneSet()
	n := 0
	for _, sec := range f.Sections {
		if len(sec) == 0 {
			continue
		}
		whole := true
		for _, id := range sec {
			if !done[id] {
				whole = false
				break
			}
		}
		if whole {
			n++
		}
	}
	return n
}

// totalLessons is the number of lessons of the course.
func (f Facts) totalLessons() int {
	total := 0
	for _, sec := range f.Sections {
		total += len(sec)
	}
	return total
}

// knownLessonsDone counts the finished lessons that are still in the course.
func (f Facts) knownLessonsDone() int {
	done := f.doneSet()
	n := 0
	for _, sec := range f.Sections {
		for _, id := range sec {
			if done[id] {
				n++
			}
		}
	}
	return n
}

// Progress is a badge with how far the person is from it.
type Progress struct {
	Def
	// Target is the number to reach (for the badge of the whole course, the number of its lessons).
	Target int
	// Value is the current number of the metric; Current is Value held at Target.
	Value   int
	Current int
	Done    bool
}

// ShowsProgress says whether the page draws a progress bar (a yes-or-no badge has none).
func (p Progress) ShowsProgress() bool { return p.Target > 1 }

// Evaluate decides every badge of the catalog for the facts.
func Evaluate(f Facts) []Progress {
	out := make([]Progress, len(Catalog))
	for i, d := range Catalog {
		target := d.Target
		if d.Metric == MAllLessons {
			target = f.totalLessons()
			if target == 0 {
				target = 1 // no course loaded: never done
			}
		}
		v := f.Value(d.Metric)
		out[i] = Progress{Def: d, Target: target, Value: v, Current: min(v, target), Done: v >= target}
	}
	return out
}

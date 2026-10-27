// Package achievements decides which badges ("Достижения") a person has earned. Nothing in the other
// packages knows about them: the badges are computed on reading from what the site already stores
// (games, ratings, puzzles, reviews, repositories, lessons, friends), and the table `achievements`
// only remembers that a badge was earned and when.
//
// The texts (names and descriptions, in Russian and English) are not here: the server sends the id,
// and the site has the words in web/src/i18n/{ru,en}/achievements.js. A test of this package checks
// that every id has both.
package achievements

// Category groups the badges on the page.
type Category string

// The categories, in the order the page shows them.
const (
	CatGames     Category = "games"
	CatBots      Category = "bots"
	CatRating    Category = "rating"
	CatPuzzles   Category = "puzzles"
	CatReview    Category = "review"
	CatOpenings  Category = "openings"
	CatLessons   Category = "lessons"
	CatCommunity Category = "community"
	CatStreaks   Category = "streaks"
)

// Categories lists them in display order.
var Categories = []Category{CatGames, CatBots, CatRating, CatPuzzles, CatReview, CatOpenings, CatLessons, CatCommunity, CatStreaks}

// Tier is the grade of a badge; the site colours the medal by it.
type Tier string

const (
	Bronze Tier = "bronze"
	Silver Tier = "silver"
	Gold   Tier = "gold"
)

// Metric names a number of the person's Facts that a badge is earned by.
type Metric string

// The metrics. A badge is earned when its metric reaches its target; a yes-or-no badge has target 1.
const (
	MGames        Metric = "games"        // finished online games
	MHumanWins    Metric = "humanWins"    // wins against people
	MRatedWins    Metric = "ratedWins"    // wins in rated games against people
	MMateWins     Metric = "mateWins"     // wins by checkmate
	MTimeWins     Metric = "timeWins"     // wins on time
	MDraws        Metric = "draws"        // drawn games
	MLongGames    Metric = "longGames"    // games of 40 moves or more
	MBotsBeaten   Metric = "botsBeaten"   // different bots beaten
	MBeatMax      Metric = "beatMax"      // 1 once Максимум is beaten
	MRating       Metric = "rating"       // the best rating in any category
	MDuels        Metric = "duels"        // puzzle duels played
	MDuelWins     Metric = "duelWins"     // puzzle duels won
	MPuzzles      Metric = "puzzles"      // puzzles solved
	MPuzzleRating Metric = "puzzleRating" // the puzzle rating
	MPointsStreak Metric = "pointsStreak" // the best series of puzzles solved in a row (points mode)
	MRush         Metric = "rush"         // the best result of the tactics storm
	MReviews      Metric = "reviews"      // reviews of games
	MAccuracy     Metric = "accuracy"     // 1 once a game was played with an accuracy of AccuracyHigh or more
	MBrilliant    Metric = "brilliant"    // 1 once a brilliant move is in the person's own game
	MRepertoires  Metric = "repertoires"  // repositories of moves
	MRepMoves     Metric = "repMoves"     // moves in the repositories
	MLearnedMoves Metric = "learnedMoves" // own moves learned to the end of the ladder
	MLessons      Metric = "lessons"      // lessons finished
	MSections     Metric = "sections"     // sections of lessons finished whole
	MAllLessons   Metric = "allLessons"   // lessons finished, for the badge of the whole course
	MFriends      Metric = "friends"      // friends
	MForumPosts   Metric = "forumPosts"   // posts on the forum
	MCorrGames    Metric = "corrGames"    // correspondence games played to the end
	MStreakDays   Metric = "streakDays"   // the best series of days with puzzles
)

// Def describes one badge.
type Def struct {
	ID       string
	Category Category
	Tier     Tier
	Metric   Metric
	Target   int
}

// Catalog is every badge, in the order of the page: by category, then roughly from easy to hard.
// The thresholds are the constants of thresholds.go.
var Catalog = []Def{
	// Партии
	{"first_game", CatGames, Bronze, MGames, 1},
	{"games_10", CatGames, Bronze, MGames, Games10},
	{"games_100", CatGames, Silver, MGames, Games100},
	{"games_500", CatGames, Gold, MGames, Games500},
	{"first_human_win", CatGames, Silver, MHumanWins, 1},
	{"rated_win", CatGames, Silver, MRatedWins, 1},
	{"mate_win", CatGames, Bronze, MMateWins, 1},
	{"time_win", CatGames, Bronze, MTimeWins, 1},
	{"first_draw", CatGames, Bronze, MDraws, 1},
	{"long_game", CatGames, Bronze, MLongGames, 1},

	// Боты
	{"first_bot_win", CatBots, Bronze, MBotsBeaten, 1},
	{"bots_3", CatBots, Bronze, MBotsBeaten, Bots3},
	{"bots_5", CatBots, Silver, MBotsBeaten, Bots5},
	{"bots_10", CatBots, Gold, MBotsBeaten, Bots10},
	{"beat_max", CatBots, Gold, MBeatMax, 1},

	// Рейтинг
	{"rating_1400", CatRating, Bronze, MRating, Rating1},
	{"rating_1600", CatRating, Silver, MRating, Rating2},
	{"rating_1800", CatRating, Gold, MRating, Rating3},
	{"rating_2000", CatRating, Gold, MRating, Rating4},
	{"first_duel", CatRating, Bronze, MDuels, 1},

	// Задачи
	{"first_puzzle", CatPuzzles, Bronze, MPuzzles, 1},
	{"puzzles_100", CatPuzzles, Silver, MPuzzles, Puzzles100},
	{"puzzles_500", CatPuzzles, Gold, MPuzzles, Puzzles500},
	{"puzzle_rating_1500", CatPuzzles, Silver, MPuzzleRating, PuzzleRating1},
	{"puzzle_rating_1800", CatPuzzles, Gold, MPuzzleRating, PuzzleRating2},
	{"points_streak_5", CatPuzzles, Bronze, MPointsStreak, Streak5},
	{"points_streak_20", CatPuzzles, Silver, MPointsStreak, Streak20},
	{"rush_20", CatPuzzles, Silver, MRush, RushScore},
	{"duel_win", CatPuzzles, Silver, MDuelWins, 1},

	// Разбор
	{"first_review", CatReview, Bronze, MReviews, 1},
	{"reviews_10", CatReview, Silver, MReviews, Reviews10},
	{"accuracy_90", CatReview, Silver, MAccuracy, 1},
	{"brilliant_move", CatReview, Gold, MBrilliant, 1},

	// Дебюты
	{"first_repertoire", CatOpenings, Bronze, MRepertoires, 1},
	{"rep_moves_50", CatOpenings, Silver, MRepMoves, RepMoves50},
	{"rep_moves_200", CatOpenings, Gold, MRepMoves, RepMoves200},
	{"learned_line", CatOpenings, Silver, MLearnedMoves, LearnedLine},

	// Уроки
	{"first_lesson", CatLessons, Bronze, MLessons, 1},
	{"lesson_section", CatLessons, Silver, MSections, 1},
	{"all_lessons", CatLessons, Gold, MAllLessons, 0}, // the target is the number of lessons of the course

	// Сообщество
	{"first_friend", CatCommunity, Bronze, MFriends, 1},
	{"friends_10", CatCommunity, Silver, MFriends, Friends10},
	{"first_post", CatCommunity, Bronze, MForumPosts, 1},
	{"corr_game", CatCommunity, Silver, MCorrGames, 1},

	// Серии
	{"streak_3", CatStreaks, Bronze, MStreakDays, Streak3Days},
	{"streak_7", CatStreaks, Silver, MStreakDays, Streak7Days},
	{"streak_30", CatStreaks, Gold, MStreakDays, Streak30Days},
}

var byID = func() map[string]Def {
	m := make(map[string]Def, len(Catalog))
	for _, d := range Catalog {
		m[d.ID] = d
	}
	return m
}()

// Lookup returns the badge with this id.
func Lookup(id string) (Def, bool) {
	d, ok := byID[id]
	return d, ok
}

package achievements

import "time"

// Every threshold of the badges is here, so that a change of difficulty is one edit.

// Games.
const (
	Games10  = 10
	Games100 = 100
	Games500 = 500
	// LongGameMoves is the number of full moves of a "long game". The game record counts half-moves:
	// a game of 40 moves has at least 79 of them (White's 40th move is the 79th).
	LongGameMoves = 40
	LongGamePlies = LongGameMoves*2 - 1
)

// Bots (there are ten of them: bots.All).
const (
	Bots3  = 3
	Bots5  = 5
	Bots10 = 10
)

// Ratings. A category counts only after MinRatedGames games: before that the rating is a rough guess
// (the first win lifts a new player by hundreds of points).
const (
	Rating1       = 1400
	Rating2       = 1600
	Rating3       = 1800
	Rating4       = 2000
	MinRatedGames = 5
)

// Puzzles. The puzzle rating counts after MinPuzzleAttempts tries, for the same reason.
const (
	Puzzles100        = 100
	Puzzles500        = 500
	PuzzleRating1     = 1500
	PuzzleRating2     = 1800
	MinPuzzleAttempts = 10
	Streak5           = 5  // puzzles solved in a row in the points mode
	Streak20          = 20 // the same, harder
	RushScore         = 20 // puzzles solved in one run of the tactics storm
)

// Reviews.
const (
	Reviews10    = 10
	AccuracyHigh = 90.0 // accuracy, in percent, of the person's own side in a game
	// AccuracyMinPlies keeps a short game (a quick resignation) from earning the badge cheaply.
	AccuracyMinPlies = 20
	// ReviewScanLimit is how many of the latest reviews are searched for a brilliant move; a newer
	// review is always among them, and the older ones were searched when they were new.
	ReviewScanLimit = 100
)

// Openings (the repository of moves).
const (
	RepMoves50  = 50
	RepMoves200 = 200
	// LearnedLine is the number of one's own moves that must be learned to the end of the ladder.
	LearnedLine = 5
)

// Community.
const Friends10 = 10

// Series of days with puzzles (the series of the statistics tab).
const (
	Streak3Days  = 3
	Streak7Days  = 7
	Streak30Days = 30
)

// How long an evaluation is reused. The profile page asks for fresh numbers a little more often than the
// notice in the corner (which polls every minute and after a change of page, and only wants to know about
// a new badge); another person's profile may be a minute old.
const (
	ProfileMaxAge = 5 * time.Second
	PollMaxAge    = 15 * time.Second
	PublicMaxAge  = 60 * time.Second
	// cacheLimit is how many people the service remembers at once.
	cacheLimit = 4096
)

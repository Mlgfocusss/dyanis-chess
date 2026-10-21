package trainer

// Every threshold of the trainer lives here, so that the rules can be read and tuned in one place.
// The trainer is rules over the person's own data: no model and no outside service.

// Reviews taken into the picture of how a person plays.
const (
	// ReviewSample is how many of the latest reviews are read (the reports are large).
	ReviewSample = 30
	// MinReviews is how many reviews with a recognised colour the picture needs; fewer and the page
	// asks for more games instead of guessing.
	MinReviews = 3
	// TrendWindow is the number of reviews in each of the two groups of the accuracy trend (the
	// latest against the ones before them); a group needs at least TrendMin to be compared.
	TrendWindow = 10
	TrendMin    = 3
)

// Phases of a game, by the half-move number and the material left (see Phases).
const (
	// OpeningPlies: up to this half-move (the 10th move) a game is in its opening, and further for as
	// long as the moves are theory (class "book").
	OpeningPlies = 20
	// EndgameMaterial: the endgame begins when the pieces of both sides together (knight and bishop 3,
	// rook 5, queen 9; pawns and kings not counted) are worth this much or less. A full board has 62;
	// queen and rook against queen and rook is 28 (still a middlegame); two rooks and a bishop against
	// two rooks is 23.
	EndgameMaterial = 26
	// MinPhaseMoves: a phase shows its accuracy only with at least this many of the person's moves in
	// it (book moves do not count), otherwise one move would make the bar.
	MinPhaseMoves = 8
)

// Costly mistakes shown on the page.
const (
	CostlyTop = 3
	// CostlyMinLoss is the least loss of win probability (points) of a move worth listing: a mistake.
	CostlyMinLoss = 10.0
	// A move classed "miss" is counted as an inaccuracy, a mistake or a blunder by its loss, with the
	// bounds of the engine's classes.
	MistakeLoss = 10.0
	BlunderLoss = 20.0
)

// The plan for the day.
const (
	// PuzzleGoal is the number of puzzles of the day (on the weakest theme, or any).
	PuzzleGoal = 5
	// RepeatCap is the most cards the plan asks to repeat in a day.
	RepeatCap = 20
	// NewMovesGoal is how many new moves of a repository the plan asks to learn when nothing is due.
	NewMovesGoal = 5
	// RecentGameDays: games of the last days are offered for a review.
	RecentGameDays = 7
	// MinReviewPlies: a shorter game is not worth a review.
	MinReviewPlies = 4
)

// Recommendations ("Что подтянуть").
const (
	// PhaseGap is how many accuracy points the weakest phase has to trail the best one by to be
	// called weak.
	PhaseGap = 8.0
	// BlundersPer100 is the rate of blunders per 100 of the person's moves that is called many.
	BlundersPer100 = 3.0
	// A puzzle theme is weak with at least ThemeMinAttempts first tries and ThemeFailPercent of them
	// failed; ThemeCards of them are listed.
	ThemeMinAttempts = 3
	ThemeFailPercent = 40
	ThemeCards       = 3
	// Games of the last DeviationDays days (at most DeviationGames) are put against the person's
	// repositories; when DeviationMinGames of them could be compared and at least DeviationPercent
	// of those left the repository at the person's own move, it is a recommendation.
	DeviationDays     = 30
	DeviationGames    = 20
	DeviationMinGames = 3
	DeviationPercent  = 30
	// MaxRepositories: at most this many repositories (the biggest) are compared with games.
	MaxRepositories = 8
	// MaxAdvice is the most cards shown; with fewer than MinAdvice from the rules a "keep going" card is added.
	MaxAdvice = 5
	MinAdvice = 2
)

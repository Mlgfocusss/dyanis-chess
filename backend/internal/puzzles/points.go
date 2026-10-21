package puzzles

// The points mode (settings → Игровой процесс → Задачи → "Собирать очки за решение задач"): a
// first try that is solved earns points, a failed one earns nothing and takes nothing away, and
// the puzzle rating does not move. These numbers are our own rules; chess.com does not publish its.
const (
	// PointsPerRating: the base is the puzzle's rating divided by this, rounded.
	PointsPerRating = 100
	// MinPoints is the least a solved puzzle earns.
	MinPoints = 1
	// BonusEvery: every this many solved puzzles in a row earn the bonus.
	BonusEvery = 5
	// Bonus is the extra points for that.
	Bonus = 1
)

// PuzzlePoints is what a solved first try earns. streak is the number of puzzles solved in a row
// before this one; usedHint halves the base (rounded down, never below MinPoints). The solution
// shown is a failure, so it never gets here.
func PuzzlePoints(puzzleRating, streak int, usedHint bool) int {
	base := max(MinPoints, (puzzleRating+PointsPerRating/2)/PointsPerRating)
	if usedHint {
		base = max(MinPoints, base/2)
	}
	if (streak+1)%BonusEvery == 0 {
		base += Bonus
	}
	return base
}

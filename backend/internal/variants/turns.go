package variants

// TurnCounter is implemented by a State whose single action can hand the turn
// on more than once: in dice chess a side whose three dice all name pieces that
// cannot move loses its whole turn at once, so after White's last move the turn
// goes to Black and comes straight back to White, and State.Turn() alone shows
// no change. The server counts a pass of the turn from this number when a State
// has it (turns done, the clock, the increment), otherwise from a change of Turn.
type TurnCounter interface {
	// TurnsPassed is how many times the turn has gone from one side to the other
	// since the game began (a skipped turn counts as passed).
	TurnsPassed() int
}

// Pure logic of the puzzle solver, with no imports, so `npm test` covers it.
//
// A puzzle from the server has steps: steps[0] is the opponent's move that
// makes the puzzle, then the person's move, the opponent's reply, and so on.
// Every step carries the position after it; the person's steps also carry
// the legal moves before them and the other moves that give checkmate (any
// mate solves a puzzle, as on Lichess).

// What the person's move `uci` means at step `idx` (an odd index):
//   { kind: "wrong" }
//   { kind: "right", fen, san, done }   the line's own move
//   { kind: "mate",  fen, san, done: true }   another move that mates
// done: the puzzle is solved with this move.
export function judgeMove(puzzle, idx, uci) {
  const step = puzzle.steps[idx];
  if (!step) return { kind: "wrong" };
  if (uci === step.uci) {
    return { kind: "right", fen: step.fen, san: step.san, done: idx >= puzzle.steps.length - 1 };
  }
  const alt = (step.alt || []).find((a) => a.uci === uci);
  if (alt) return { kind: "mate", fen: alt.fen, san: alt.san, done: true };
  return { kind: "wrong" };
}

// The legal moves for the board at step idx, in the shape Board.jsx takes.
export function legalFor(puzzle, idx) {
  const step = puzzle.steps[idx];
  return ((step && step.legal) || []).map((uci) => ({ uci }));
}

// The square to hint at: where the piece of the right move stands.
export function hintSquare(puzzle, idx) {
  const step = puzzle.steps[idx];
  return step ? step.uci.slice(0, 2) : null;
}

// Squares of a move, for the last-move highlight.
export function squaresOf(uci) {
  return uci ? { from: uci.slice(0, 2), to: uci.slice(2, 4) } : null;
}

// "+18" / "-12" / "0" for a change of rating.
export function signed(n) {
  return n > 0 ? `+${n}` : n < 0 ? `−${-n}` : "0";
}

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

// "2026-10-16" as "16 октября" (the puzzle of the day changes at midnight UTC).
export function dayLabel(date) {
  const d = new Date(`${date}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("ru-RU", { day: "numeric", month: "long", timeZone: "UTC" });
}

// "1 очко", "2 очка", "5 очков" (the points mode).
export function pointsText(n) {
  const m10 = Math.abs(n) % 10;
  const m100 = Math.abs(n) % 100;
  const word = m10 === 1 && m100 !== 11 ? "очко" : m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14) ? "очка" : "очков";
  return `${n} ${word}`;
}

// "+18" / "-12" / "0" for a change of rating.
export function signed(n) {
  return n > 0 ? `+${n}` : n < 0 ? `−${-n}` : "0";
}

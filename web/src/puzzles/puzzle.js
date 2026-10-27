import { locale, tr } from "../i18n/index.js";
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
  return d.toLocaleDateString(locale(), { day: "numeric", month: "long", timeZone: "UTC" });
}

// "1 очко", "2 очка", "5 очков" (the points mode).
export function pointsText(n) {
  return tr("puzzles.points", { n });
}

// "+18" / "-12" / "0" for a change of rating.
export function signed(n) {
  return n > 0 ? `+${n}` : n < 0 ? `−${-n}` : "0";
}

// What the panel of the rated puzzles shows as numbers. The rated puzzles always count the rating
// and the points; the setting "Собирать очки" (pointsMain) only decides which one is the main
// number: the points (the rating is hidden) or the rating (the points are the modest second one).
//   you      { rating, attempts, solved } or null     pts     { total, streak, best, today } or null
//   result   the server's answer for the first try, or null while the puzzle is open
//   ratingVisible: the setting "Всегда показывать рейтинг"
// Returns { main, side }: each is { kind: "rating" | "points", value, change, from, streak, today } or null.
// change is the move of this try (null when nothing happened: no answer yet, or a repeated puzzle).
export function scoreView({ pointsMain, ratingVisible, you, pts, result }) {
  const counted = !!result?.counted;
  const rating = you
    ? { kind: "rating", value: you.rating, change: counted ? result.after - result.before : null, from: counted ? result.before : null }
    : null;
  const points = pts
    ? { kind: "points", value: pts.total, change: counted ? result.points?.gained ?? 0 : null, streak: pts.streak, today: pts.today }
    : null;
  if (pointsMain) return { main: points, side: rating && ratingVisible ? rating : null }; // the setting only puts the points first
  if (rating && ratingVisible) return { main: rating, side: points };
  return { main: points, side: null }; // the rating is hidden: the points are what there is to show
}

// What Dionis says in the panel. phase is the page's: loading | intro | solving | reply | solved | shown.
// Returns { kind (the short line), text, tone: "" | "ok" | "bad" | "warn" }.
export function coachSay({ loading, phase, failed, idx, mySide }) {
  const side = mySide === "white" ? tr("puzzleJs.4") : tr("puzzleJs.5");
  if (loading || phase === "loading") return { kind: tr("puzzleJs.6"), text: tr("puzzleJs.7"), tone: "" };
  if (phase === "intro") return { kind: tr("puzzleJs.8"), text: tr("puzzleJs.9"), tone: "" };
  if (phase === "solved") {
    return failed
      ? { kind: tr("puzzleJs.10"), text: tr("puzzleJs.11"), tone: "warn" }
      : { kind: tr("puzzleJs.12"), text: tr("puzzleJs.13"), tone: "ok" };
  }
  if (phase === "shown") return { kind: tr("puzzleJs.14"), text: tr("puzzleJs.15"), tone: "warn" };
  if (failed) return { kind: tr("puzzleJs.16"), text: tr("puzzleJs.17"), tone: "bad" };
  if (idx > 1) return { kind: tr("puzzleJs.12"), text: tr("puzzleJs.18"), tone: "ok" };
  return { kind: tr("gameRoom.41"), text: tr("puzzleJs.20", { side }), tone: "" };
}

// The keys of the puzzle page: Enter, Space or N open the next puzzle, H asks for a hint, S shows the
// solution (the Russian layout's т, р and ы work too). Anything else is null.
export function keyAction(key) {
  if (key === "Enter" || key === " ") return "next";
  if (key === "ArrowLeft") return "back";
  if (key === "ArrowRight") return "forward";
  if (key === "Home") return "first";
  if (key === "End") return "last";
  const k = String(key).toLowerCase();
  if (k === "n" || k === "т") return "next";
  if (k === "h" || k === "р") return "hint";
  if (k === "s" || k === "ы") return "solution";
  return null;
}

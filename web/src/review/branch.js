// Pure helpers for the branch a person plays during a review, with no imports
// so that `npm test` covers them.

// The classes of internal/analysis for a move that is not the engine's choice,
// from the win probability the mover lost (the same thresholds as
// analysis.DefaultThresholds).
export function classOfLoss(loss) {
  if (loss >= 20) return "blunder";
  if (loss >= 10) return "mistake";
  if (loss >= 5) return "inaccuracy";
  if (loss >= 2) return "good";
  return "excellent";
}

// Win probability points the mover lost by a move. before and after are the
// engine's verdicts on the positions before and after it (winPct is White's
// chance); color is the mover, "white" or "black". null when either is missing.
export function moverLoss(before, after, color) {
  if (!before || !after || typeof before.winPct !== "number" || typeof after.winPct !== "number") return null;
  const sign = color === "white" ? 1 : -1;
  return Math.max(0, sign * (before.winPct - after.winPct));
}

// "w" or "b" from a FEN: who moves in it.
export function sideOf(fen) {
  return String(fen).split(" ")[1] === "b" ? "black" : "white";
}

// How a branch move is judged: { cls, loss } or null while an evaluation is
// still missing. Playing the engine's own move is "best".
export function judgeBranchMove(step, before, after, color) {
  const loss = moverLoss(before, after, color);
  if (loss === null) return null;
  if (before.bestUci && before.bestUci === step.uci) return { cls: "best", loss: 0 };
  return { cls: classOfLoss(loss), loss };
}

// The moves of a branch with their numbers, as written in a score:
// [{ san, label: "5." | "5..." | "" }]. A number is shown before each White
// move and before Black's first move of the branch. baseFen is the position
// the branch starts from.
export function numberedSans(baseFen, steps) {
  const f = String(baseFen).split(/\s+/);
  let black = f[1] === "b";
  let no = Number(f[5]) > 0 ? Number(f[5]) : 1;
  return steps.map((s, i) => {
    let label = "";
    if (!black) label = `${no}.`;
    else if (i === 0) label = `${no}...`;
    if (black) no++;
    black = !black;
    return { san: s.san, label };
  });
}

// "1. e4 e5 2. Nf3" for a list of numbered moves.
export function lineText(numbered) {
  return numbered.map((m) => (m.label ? `${m.label} ${m.san}` : m.san)).join(" ");
}

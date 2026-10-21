// Pure logic of the engine's arrows on the board of a review (no imports, so that `npm test` covers it).
// The three best lines of a position (MultiPV) are drawn as arrows of their first moves: the best line
// thickest and brightest, the second thinner, the third thinnest, and each thinner and paler the more of
// the chance of winning it gives away against the best one. The red arrow of Dionis (the reply that
// punishes a bad move) takes the place of the green one when it is the same move.

export const MAX_ARROWS = 3;
// The width of an arrow (in squares of the board) and its opacity by the rank of the line, before the
// loss against the best line is taken into account.
export const BASE_WIDTH = [0.24, 0.18, 0.13];
export const BASE_OPACITY = [0.95, 0.86, 0.76];
// A line this many points of win probability worse than the best is as thin and pale as it gets.
export const LOSS_SPAN = 30;
// How much of the width and of the opacity the worst loss takes away: the three arrows stay readable.
export const WIDTH_LOSS = 0.3;
export const OPACITY_LOSS = 0.15;

const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));

// The side to move in a FEN: "white" or "black".
function sideOf(fen) {
  return String(fen).split(" ")[1] === "b" ? "black" : "white";
}

// The chance of winning of the side to move, from the White's winPct.
export function moverWin(winPct, side) {
  return side === "white" ? winPct : 100 - winPct;
}

// Width and opacity of the arrow of the line of the given rank (0 = the best), losing `loss` points of win
// probability against the best line.
export function arrowStyle(rank, loss = 0) {
  const r = clamp(rank, 0, BASE_WIDTH.length - 1);
  const k = clamp(loss / LOSS_SPAN, 0, 1);
  return {
    width: +(BASE_WIDTH[r] * (1 - WIDTH_LOSS * k)).toFixed(4),
    opacity: +(BASE_OPACITY[r] * (1 - OPACITY_LOSS * k)).toFixed(4),
  };
}

// "e2e4" -> { from, to }; null for anything shorter than four characters.
export function squaresOf(uci) {
  return uci && uci.length >= 4 ? { from: uci.slice(0, 2), to: uci.slice(2, 4) } : null;
}

// The arrows to draw for the position fen, from the back to the front (the best line is last, so it is on top):
//   lines    the engine's lines [{bestUci, winPct}] best first, or null while they are being worked out
//   first    the line the review already has ({bestUci, winPct}), used until the lines come
//   threat   Dionis's red arrow: the reply that punishes the last move, drawn from first.bestUci
//   enabled  the "engine arrows" setting; off leaves only the red arrow
// Returns [{ uci, from, to, kind: "best" | "second" | "third" | "threat", width, opacity }]. Two lines that
// start with the same move give one arrow, the better line's; the red one takes the move over from the
// green one.
export function pickArrows({ fen, lines, first, threat = false, enabled = true }) {
  const out = [];
  const seen = new Set();
  const add = (uci, kind, rank, loss) => {
    const sq = squaresOf(uci);
    if (!sq || seen.has(uci)) return;
    seen.add(uci);
    out.push({ uci, ...sq, kind, ...arrowStyle(rank, loss) });
  };

  if (threat && first?.bestUci) add(first.bestUci, "threat", 0, 0);
  if (enabled) {
    const side = sideOf(fen);
    const list = lines && lines.length ? lines : first?.bestUci ? [first] : [];
    const best = list.length && typeof list[0].winPct === "number" ? moverWin(list[0].winPct, side) : null;
    const kinds = ["best", "second", "third"];
    list.slice(0, MAX_ARROWS).forEach((l, i) => {
      const mine = typeof l.winPct === "number" && best !== null ? moverWin(l.winPct, side) : null;
      add(l.bestUci, kinds[i], i, mine === null ? 0 : Math.max(0, best - mine));
    });
  }
  return out.reverse();
}

// The squares an arrow runs through, in units of one square (the board is 8 x 8, the corner a8 for White at
// the bottom is (0, 0)): the centres of the squares it starts and ends in. orientation is the side at the bottom.
export function squareCentre(square, orientation) {
  const f = square.charCodeAt(0) - 97;
  const r = Number(square[1]) - 1;
  return orientation === "black" ? [7 - f + 0.5, r + 0.5] : [f + 0.5, 7 - r + 0.5];
}

// The shape of an arrow: the polyline of its shaft (a knight's move bends, the long way first) cut short
// where the head begins, and the three corners of the head. Units are squares; width is the shaft's.
export function arrowShape(from, to, orientation, width) {
  const a = squareCentre(from, orientation);
  const b = squareCentre(to, orientation);
  const dx = b[0] - a[0];
  const dy = b[1] - a[1];
  const knight = (Math.abs(dx) === 1 && Math.abs(dy) === 2) || (Math.abs(dx) === 2 && Math.abs(dy) === 1);
  const path = [a];
  if (knight) path.push(Math.abs(dx) > Math.abs(dy) ? [b[0], a[1]] : [a[0], b[1]]);
  path.push(b);

  // the last segment carries the head; the tip stops a little short of the centre of the square
  const p = path[path.length - 2];
  const q = path[path.length - 1];
  const len = Math.hypot(q[0] - p[0], q[1] - p[1]);
  const ux = (q[0] - p[0]) / len;
  const uy = (q[1] - p[1]) / len;
  const headLen = Math.min(width * 2.2, len * 0.75);
  const headHalf = width * 1.3;
  const tipBack = Math.min(0.12, len * 0.2);
  const tip = [q[0] - ux * tipBack, q[1] - uy * tipBack];
  const base = [tip[0] - ux * headLen, tip[1] - uy * headLen];
  const shaft = [...path.slice(0, -1), base];
  const head = [tip, [base[0] - uy * headHalf, base[1] + ux * headHalf], [base[0] + uy * headHalf, base[1] - ux * headHalf]];
  return { shaft, head };
}

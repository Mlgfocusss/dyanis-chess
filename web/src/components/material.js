// Material count of a position, with no imports so that `npm test` covers it.

export const VALUES = { p: 1, n: 3, b: 3, r: 5, q: 9 };
const START = { p: 8, n: 2, b: 2, r: 2, q: 1 };
// Shown from the most valuable captured piece to the least.
export const ORDER = ["q", "r", "b", "n", "p"];

// What each side has taken from the other, and who is ahead.
//   white / black  the pieces of that colour missing from the board, e.g.
//                  white: { q: 0, r: 1, b: 0, n: 1, p: 2 } is what Black has lost
//                  (so White took them)
//   diff           material points White leads by (negative: Black leads)
// A promoted pawn is not counted as a lost pawn: the extra pieces of a side
// cancel as many of its missing pawns, so a queen made from a pawn shows no
// captured pawn. The lead counts what is on the board, so it is right too.
export function materialOf(fen) {
  const placement = String(fen || "").split(" ")[0];
  const on = { w: { p: 0, n: 0, b: 0, r: 0, q: 0 }, b: { p: 0, n: 0, b: 0, r: 0, q: 0 } };
  for (const c of placement) {
    const k = c.toLowerCase();
    if (!(k in VALUES)) continue;
    on[c === k ? "b" : "w"][k]++;
  }
  const lost = (side) => {
    const out = {};
    let promoted = 0;
    for (const k of ORDER) {
      if (k !== "p") promoted += Math.max(0, on[side][k] - START[k]);
    }
    for (const k of ORDER) {
      let n = Math.max(0, START[k] - on[side][k]);
      if (k === "p") n = Math.max(0, n - promoted);
      out[k] = n;
    }
    return out;
  };
  const total = (side) => ORDER.reduce((s, k) => s + on[side][k] * VALUES[k], 0);
  return { white: lost("b"), black: lost("w"), diff: total("w") - total("b") };
}

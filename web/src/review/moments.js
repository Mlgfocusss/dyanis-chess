// Pure helpers of the review screen (no imports, so that `npm test` covers them): the moments of a game
// worth stopping at, the table of how many moves of each class each player made, the marks on the
// evaluation chart, and the addresses of a review and of the analysis board.

// The classes "next mistake" stops at: the moves that cost something. Inaccuracies are too frequent to
// walk through one by one; they stay in the table and on the list.
export const MISTAKE_CLASSES = ["blunder", "mistake", "miss"];
// The classes that get a mark on the chart: what turned the game, for better and for worse.
export const MARK_CLASSES = ["brilliant", "great", "miss", "mistake", "blunder"];

// The 1-based plies of the moves whose class is in kinds, in game order; side ("white" | "black")
// keeps one player's moves only.
export function momentPlies(moves, { kinds = MISTAKE_CLASSES, side = null } = {}) {
  const out = [];
  for (const m of moves || []) {
    if (!kinds.includes(m.class)) continue;
    if (side && m.color !== side) continue;
    out.push(m.ply);
  }
  return out;
}

// The moment after ply (dir = 1) or before it (dir = -1), or null when there is none that way.
export function stepMoment(plies, ply, dir) {
  if (dir > 0) {
    for (const p of plies) if (p > ply) return p;
    return null;
  }
  for (let i = plies.length - 1; i >= 0; i--) if (plies[i] < ply) return plies[i];
  return null;
}

// Marks for the chart: [{ply, cls}] for the moves of the classes that turn a game.
export function chartMarks(moves) {
  return (moves || []).filter((m) => MARK_CLASSES.includes(m.class)).map((m) => ({ ply: m.ply, cls: m.class }));
}

// The rows of the summary table: [{cls, white, black}] for the classes either player has at least one
// move of, in the order given (best first). counts is the report's [white, black] maps.
export function summaryRows(counts, order) {
  const w = counts?.[0] || {};
  const b = counts?.[1] || {};
  return order.filter((k) => (w[k] || 0) + (b[k] || 0) > 0).map((k) => ({ cls: k, white: w[k] || 0, black: b[k] || 0 }));
}

// "Ошибок: 3" style total of the moves that cost something, for one player's map of counts.
export function errorCount(counts) {
  return ["inaccuracy", "mistake", "blunder", "miss"].reduce((n, k) => n + (counts?.[k] || 0), 0);
}

// The move written as in a score: "7.Ng5", "7...Qxg5".
export function moveLabel(move) {
  if (!move) return "";
  return `${Math.ceil(move.ply / 2)}.${move.color === "black" ? ".." : ""}${move.san}`;
}

// A FEN in an address: spaces become underscores (as on lichess), slashes stay.
export function encodeFen(fen) {
  return String(fen).trim().replace(/\s+/g, "_");
}
export function decodeFen(text) {
  return String(text || "").replace(/_/g, " ").trim();
}

// The part of the address after "#analysis/": the position the board starts from, or "".
export function fenFromHash(hash) {
  const h = String(hash || "").replace(/^#/, "");
  if (!h.startsWith("analysis/")) return "";
  let rest = h.slice("analysis/".length);
  try {
    rest = decodeURIComponent(rest);
  } catch {
    // keep it as it is
  }
  return looksLikeFen(decodeFen(rest)) ? decodeFen(rest) : "";
}

// Whether the text is a position (six fields, eight ranks) rather than a game's moves. Only the shape
// is checked here; the server knows the rules and refuses a wrong one.
export function looksLikeFen(text) {
  const f = String(text || "").trim().split(/\s+/);
  if (f.length < 4 || f.length > 6) return false;
  const ranks = f[0].split("/");
  if (ranks.length !== 8) return false;
  if (!ranks.every((r) => /^[pnbrqkPNBRQK1-8]+$/.test(r))) return false;
  return f[1] === "w" || f[1] === "b";
}

// The address a finished review can be opened at, whatever the page the site stands on.
export function reviewUrl(origin, pathname, id) {
  return `${origin}${pathname}#review/${id}`;
}

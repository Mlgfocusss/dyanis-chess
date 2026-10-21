// Pockets of Crazyhouse and Bughouse: the pure part (covered by `npm test`). A drop is the action "N@f3"
// (the server's State.Apply takes it); the pocket's content arrives in view.extra as
// { white: { Q: 1, P: 2 }, black: {...}, promoted: ["a8"] }.

/** The kinds a pocket holds, in the order it shows them. */
export const POCKET_ORDER = ["Q", "R", "B", "N", "P"];

const DROP = /^([PNBRQ])@([a-h][1-8])$/;

/** "N@f3" -> { kind: "N", square: "f3" }, or null for anything that is not a drop. */
export function parseDrop(action) {
  const m = DROP.exec(action || "");
  return m ? { kind: m[1], square: m[2] } : null;
}

export const dropAction = (kind, square) => `${kind}@${square}`;

/** The squares the piece of that kind may be dropped on, from the legal actions. */
export function dropSquares(legal, kind) {
  const out = [];
  for (const a of legal || []) {
    const d = parseDrop(a);
    if (d && d.kind === kind) out.push(d.square);
  }
  return out;
}

/** The kinds that have at least one legal drop. */
export function droppableKinds(legal) {
  const out = new Set();
  for (const a of legal || []) {
    const d = parseDrop(a);
    if (d) out.add(d.kind);
  }
  return out;
}

/** How many pieces of each kind a side holds: { Q: 0, R: 1, ... } for every kind of POCKET_ORDER. */
export function pocketCounts(extra, side) {
  const raw = (extra && extra[side]) || {};
  return Object.fromEntries(POCKET_ORDER.map((k) => [k, Math.max(0, Number(raw[k]) || 0)]));
}

/** The number of pieces in a pocket. */
export const pocketTotal = (counts) => POCKET_ORDER.reduce((n, k) => n + (counts[k] || 0), 0);

/** "wN" -> "N" (the kind of a piece code the board hands over when a piece is dropped from outside). */
export const kindOfCode = (code) => (typeof code === "string" && code.length === 2 ? code[1].toUpperCase() : "");

/** The side a piece code belongs to: "wN" -> "white". */
export const sideOfCode = (code) => (code?.[0] === "w" ? "white" : code?.[0] === "b" ? "black" : "");

/** { sq: "dot" } for the squares a drop of the kind may go to (the board's extraDots), or undefined for none. */
export function dropDots(legal, kind) {
  if (!kind) return undefined;
  const dots = {};
  for (const sq of dropSquares(legal, kind)) dots[sq] = "dot";
  return Object.keys(dots).length ? dots : undefined;
}

/** { sq: "sq-promoted" } for the promoted pieces (a taken one goes back to a pocket as a pawn). */
export function promotedClasses(extra) {
  const out = {};
  for (const sq of extra?.promoted || []) out[sq] = "sq-promoted";
  return out;
}

/**
 * What a click on a square does while a pocket piece is chosen: the action to send, or null when the
 * click is not a drop (the board then treats it as an ordinary click).
 */
export function dropClick(legal, kind, square) {
  if (!kind) return null;
  const a = dropAction(kind, square);
  return (legal || []).includes(a) ? a : null;
}

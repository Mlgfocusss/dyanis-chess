import { tr } from "../i18n/index.js";
// Pure helpers for a repertoire's tree of moves, with no imports so that
// `npm test` covers them. A move is { id, parent, uci, san, fen, ply, mine, box, due, ... }
// as the server sends it; parent 0 means a first move from the start position.

export const START_FEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

// A move is "learned" after this many steps of the ladder (as on the server).
export const LEARNED_BOX = 5;

// byId: Map id -> move; children: Map parent id -> [moves] (in the order added).
export function buildTree(moves) {
  const byId = new Map();
  const children = new Map();
  for (const m of moves) {
    byId.set(m.id, m);
    if (!children.has(m.parent)) children.set(m.parent, []);
    children.get(m.parent).push(m);
  }
  return { byId, children };
}

// The moves from the first one up to and including `id` ([] for 0).
export function pathTo(tree, id) {
  const out = [];
  for (let m = tree.byId.get(id); m; m = tree.byId.get(m.parent)) out.push(m);
  return out.reverse();
}

// The position after the move `id` (the start position for 0).
export function fenAt(tree, id) {
  return tree.byId.get(id)?.fen || START_FEN;
}

// The repertoire's move that follows `id` with the move `uci`, if there is one.
export function childWith(tree, id, uci) {
  return (tree.children.get(id) || []).find((m) => m.uci === uci) || null;
}

// How a move looks in the trainer's terms, for a chip:
//   "opponent"  the opponent's move, nothing to learn
//   "new"       mine, never asked yet
//   "due"       mine, asked before and due now
//   "learning"  mine, asked before, rests
//   "learned"   mine, passed enough of the ladder
export function statusOf(move, now = Date.now()) {
  if (!move.mine) return "opponent";
  if (!move.due) return "new";
  if (move.box >= LEARNED_BOX) return "learned";
  return Date.parse(move.due) <= now ? "due" : "learning";
}

// "1. e4 e5 2. Nf3" for a path of moves. A line that starts with Black's move
// (a repertoire of Black does not, but a path may begin in the middle) gets "1... e5".
export function lineText(path) {
  return path
    .map((m, i) => {
      const no = Math.ceil(m.ply / 2);
      if (m.ply % 2 === 1) return `${no}. ${m.san}`;
      return i === 0 ? `${no}... ${m.san}` : m.san;
    })
    .join(" ");
}

// Whose turn it is to move after `id`, from the repertoire's colour:
// "mine" or "opponent".
export function whoMoves(color, id, tree) {
  const ply = tree.byId.get(id)?.ply || 0; // plies played
  const whiteToMove = ply % 2 === 0;
  return whiteToMove === (color === "white") ? "mine" : "opponent";
}

// The share of the person's own moves that are learned, 0..100.
export function learnedPercent(counts) {
  return counts.mine ? Math.round((100 * counts.learned) / counts.mine) : 0;
}

// "через 3 ч", "завтра", "через 5 дн.": when to come back, in words.
export function whenText(iso, now = Date.now()) {
  const ms = Date.parse(iso) - now;
  if (!(ms > 0)) return tr("tree.1");
  const min = Math.round(ms / 60000);
  if (min < 60) return tr("tree.2", { p: Math.max(min, 1) });
  const h = Math.round(min / 60);
  if (h < 24) return tr("tree.3", { h });
  const d = Math.round(h / 24);
  return d === 1 ? tr("tree.4") : tr("tree.5", { d });
}

// The same figures the server counts, from the moves in hand, so that the page
// stays right after a move is added or deleted without asking again.
export function countsOf(moves, now = Date.now()) {
  const c = { moves: moves.length, mine: 0, new: 0, due: 0, learned: 0 };
  for (const m of moves) {
    if (!m.mine) continue;
    c.mine++;
    if (!m.due) c.new++;
    else if (Date.parse(m.due) <= now) c.due++;
    if (m.due && m.box >= LEARNED_BOX) c.learned++;
  }
  return c;
}

// The lines that end with the opponent's move: the person has no answer there
// yet. Returns those last moves.
export function gapsOf(tree) {
  const out = [];
  for (const m of tree.byId.values()) {
    if (!m.mine && !(tree.children.get(m.id) || []).length) out.push(m);
  }
  return out;
}

// The moves of the subtree below `id`, `id` included.
export function subtreeIds(tree, id) {
  const out = [];
  const stack = [id];
  while (stack.length) {
    const cur = stack.pop();
    out.push(cur);
    for (const c of tree.children.get(cur) || []) stack.push(c.id);
  }
  return out;
}

import { tr } from "../i18n/index.js";
// Pure helpers for a repertoire's tree of moves, with no imports so that
// `npm test` covers them. A move is { id, parent, uci, san, fen, ply, mine, box, due, ... }
// as the server sends it; parent 0 means a first move from the start position.

export const START_FEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

// A move is "learned" after this many steps of the ladder (as on the server).
export const LEARNED_BOX = 5;

// Transpositions. The tree keeps paths, but one position can be reached by different orders of
// moves (1.d4 Nf6 2.c4 e6 and 1.c4 e6 2.d4 Nf6). What the moves of the repository mean is decided
// by the position: nodes in the same position are one "class", and the replies known in a class
// are the replies of all its nodes. The same rule as the server's (backend/internal/repertoire/positions.go).

// The position of a FEN: the pieces, the side to move, the castling rights, and the en passant
// square only when a pawn can really take on it. The move counters are left out.
export function posKey(fen) {
  const f = String(fen || "").trim().split(/\s+/);
  if (f.length < 2) return f.join(" ");
  let ep = f[3] || "-";
  if (ep !== "-" && !epCapturable(f[0], f[1], ep)) ep = "-";
  return `${f[0]} ${f[1]} ${f[2] || "-"} ${ep}`;
}

function epCapturable(board, turn, ep) {
  if (ep.length !== 2) return false;
  const file = ep.charCodeAt(0) - 97;
  const rank = ep.charCodeAt(1) - 48;
  if (file < 0 || file > 7) return false;
  let row;
  let pawn;
  if (turn === "b" && rank === 3) [row, pawn] = [4, "p"];
  else if (turn === "w" && rank === 6) [row, pawn] = [3, "P"];
  else return false;
  const ranks = board.split("/");
  if (ranks.length !== 8) return false;
  const squares = [];
  for (const c of ranks[row]) {
    if (c >= "1" && c <= "8") for (let n = 0; n < Number(c); n++) squares.push(".");
    else squares.push(c);
  }
  if (squares.length !== 8) return false;
  return (file > 0 && squares[file - 1] === pawn) || (file < 7 && squares[file + 1] === pawn);
}

const START_KEY = posKey(START_FEN);

// byId: Map id -> move; children: Map parent id -> [moves] (in the order added);
// keyOf: Map id -> the position after the move (0: the start position);
// classes: Map position -> [ids of the nodes standing in it, 0 first when it is the start, then by id].
export function buildTree(moves) {
  const byId = new Map();
  const children = new Map();
  const keyOf = new Map([[0, START_KEY]]);
  const classes = new Map([[START_KEY, [0]]]);
  for (const m of moves) {
    byId.set(m.id, m);
    if (!children.has(m.parent)) children.set(m.parent, []);
    children.get(m.parent).push(m);
    const k = m.fen ? posKey(m.fen) : `#${m.id}`;
    keyOf.set(m.id, k);
    if (!classes.has(k)) classes.set(k, []);
    classes.get(k).push(m.id);
  }
  for (const ids of classes.values()) ids.sort((a, b) => a - b);
  return { byId, children, keyOf, classes };
}

// Whether two nodes stand in the same position.
export function samePosition(tree, a, b) {
  return a === b || (tree.keyOf.get(a) !== undefined && tree.keyOf.get(a) === tree.keyOf.get(b));
}

// The other nodes in the position of `id`: the same position reached by other orders of moves.
export function twinsOf(tree, id) {
  const k = tree.keyOf.get(id);
  return (k && tree.classes.get(k) ? tree.classes.get(k) : []).filter((x) => x !== id);
}

// The node that stands for a position in lists: the fewest plies played, then the smallest id.
export function repOf(tree, id) {
  const ids = tree.classes.get(tree.keyOf.get(id)) || [id];
  let best = ids[0];
  for (const x of ids) if ((tree.byId.get(x)?.ply || 0) < (tree.byId.get(best)?.ply || 0)) best = x;
  return best;
}

// The moves the repository has in the position of `id`: the node's own children first, then
// those of the other nodes in the same position (one for each move). A move whose `parent` is
// not `id` was found on another branch.
export function repliesAt(tree, id) {
  const own = tree.children.get(id) || [];
  const twins = twinsOf(tree, id);
  if (!twins.length) return own;
  const out = [...own];
  const seen = new Set(own.map((m) => m.uci));
  for (const t of twins) {
    for (const m of tree.children.get(t) || []) {
      if (!seen.has(m.uci)) {
        seen.add(m.uci);
        out.push(m);
      }
    }
  }
  return out;
}

// The repository's move after `id` with the move `uci`, on this branch or another one.
export function replyWith(tree, id, uci) {
  return repliesAt(tree, id).find((m) => m.uci === uci) || null;
}

// The line of the first node of another branch that stands in the same position as `id`,
// the one that has the continuation (or null): where "the same position" is described to the person.
export function transposedFrom(tree, id) {
  if ((tree.children.get(id) || []).length) return null;
  for (const t of twinsOf(tree, id)) {
    if ((tree.children.get(t) || []).length) return t;
  }
  return null;
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

// The cards of the person: his own moves, where the same move in the same position (reached by
// another order of moves) is one card. Returns [{ rep, members: [ids] }] in the order of the first node;
// the rep is the most asked node, then the first added (as on the server).
export function cardsOf(tree) {
  const index = new Map();
  const out = [];
  for (const m of tree.byId.values()) {
    if (!m.mine) continue;
    const k = `${tree.keyOf.get(m.parent) ?? `#${m.parent}`}|${m.uci}`;
    const card = index.get(k);
    if (!card) {
      const fresh = { rep: m, members: [m.id] };
      index.set(k, fresh);
      out.push(fresh);
      continue;
    }
    card.members.push(m.id);
    if ((m.reviews || 0) > (card.rep.reviews || 0)) card.rep = m;
  }
  return out;
}

// The same figures the server counts, from the moves in hand, so that the page
// stays right after a move is added or deleted without asking again. A move in the same
// position reached by another order of moves counts once.
export function countsOf(moves, now = Date.now()) {
  const tree = buildTree(moves);
  const edges = new Set();
  for (const m of moves) edges.add(`${tree.keyOf.get(m.parent) ?? `#${m.parent}`}|${m.uci}`);
  const c = { moves: edges.size, mine: 0, new: 0, due: 0, learned: 0 };
  for (const { rep: m } of cardsOf(tree)) {
    c.mine++;
    if (!m.due) c.new++;
    else if (Date.parse(m.due) <= now) c.due++;
    if (m.due && m.box >= LEARNED_BOX) c.learned++;
  }
  return c;
}

// The lines that end with the opponent's move: the person has no answer there
// yet (neither in the same position reached by another order of moves). Returns those last
// moves, one for each such position.
export function gapsOf(tree) {
  const out = [];
  const seen = new Set();
  for (const m of tree.byId.values()) {
    if (m.mine) continue;
    const k = tree.keyOf.get(m.id);
    if (seen.has(k) || repliesAt(tree, m.id).length) continue;
    seen.add(k);
    out.push(tree.byId.get(repOf(tree, m.id)));
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

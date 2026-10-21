// Blindfold chess without React, so `npm test` covers it: what the clicks mean (the legal actions are all the
// board knows: the position is hidden), the words about the last move, the board that is drawn, and the
// person's own settings of the room. The rules and the hiding live in backend/internal/variants/blindfold.go.
import { isBoardMove } from "../describe.js";

// ------------------------------------------------------------------ clicks

// The squares that have a move to make from them: the only squares a click can pick.
export function sourcesOf(legal) {
  const out = new Set();
  for (const a of legal || []) if (isBoardMove(a)) out.add(a.slice(0, 2));
  return out;
}

// The squares the piece on `from` can go to (a promotion counts once).
export function targetsOf(legal, from) {
  const out = new Set();
  for (const a of legal || []) if (isBoardMove(a) && a.startsWith(from)) out.add(a.slice(2, 4));
  return [...out];
}

const ORDER = "qrbn";
// The letters a pawn going from `from` to `to` may become, queen first, or null when it is no promotion.
export function promotionLetters(legal, from, to) {
  const out = [];
  for (const a of legal || []) if (isBoardMove(a) && a.length === 5 && a.startsWith(from + to)) out.push(a[4]);
  return out.length ? out.sort((x, y) => ORDER.indexOf(x) - ORDER.indexOf(y)) : null;
}

export const PROMOTION_NAMES = { q: "Ферзь", r: "Ладья", b: "Слон", n: "Конь" };

// What a click on `square` does, given the half-made move in `ui` ({ from }): one of
//   { type: "select", from }   the square is picked as the start
//   { type: "clear" }          the pick is dropped
//   { type: "move", action }   a complete move
//   { type: "promote", from, to, letters }   the pawn needs its piece
//   { type: "none", empty: true }   nothing to move from here
export function clickResult(legal, ui, square) {
  const sources = sourcesOf(legal);
  const from = ui?.from;
  if (from) {
    if (square === from) return { type: "clear" };
    if (targetsOf(legal, from).includes(square)) {
      const letters = promotionLetters(legal, from, square);
      return letters ? { type: "promote", from, to: square, letters } : { type: "move", action: from + square };
    }
    if (sources.has(square)) return { type: "select", from: square };
    return { type: "clear" };
  }
  return sources.has(square) ? { type: "select", from: square } : { type: "none", empty: true };
}

// ------------------------------------------------------------------- words

// The last move in words for the strip above the board. `moves` are the records of the snapshot,
// `you` is "white", "black" or "" (a spectator).
export function lastMoveLine(moves, you, ended = false) {
  if (!moves || moves.length === 0) return "";
  const m = moves[moves.length - 1];
  const san = m.san || "";
  const who = !you ? (m.side === "white" ? "Белые сыграли" : "Чёрные сыграли") : m.side === you ? "Вы сыграли" : "Соперник сыграл";
  let line = `${who} ${san}`;
  if (san.endsWith("#")) line += ": мат";
  else if (san.endsWith("+")) line += ": шах";
  if (!ended && you && m.side === you) line += ". Ждём ответа";
  return line;
}

// The number of the move, as chess writes it: "5." for White's fifth, "5..." for Black's.
export function moveNumber(moves) {
  const n = moves?.length || 0;
  if (!n) return "";
  return n % 2 === 1 ? `${(n + 1) / 2}.` : `${n / 2}...`;
}

// ------------------------------------------------------------------- board

// The board the room draws: while the game is on and hidden, nothing (the server sends no position then; with
// peeking allowed it is in `extra.board`); while the person peeks, or when a spectator may look (peeking is
// allowed), that position; at the end, the snapshot's. null: leave the room's choice.
export function boardToShow({ extra, you, peeking }) {
  if (!extra || !extra.blind) return null; // over (or not a blindfold snapshot): as it comes
  if (extra.board && (you ? peeking : true)) return extra.board;
  return EMPTY;
}
const EMPTY = Object.freeze({});

// The squares to light on the board: { from, to } or null. While the game is on a player sees the
// opponent's last move (when `oppLast` is on; the player's own move is not lit: it is known), a spectator
// sees the last move whoever made it. The server sends only the two squares (no piece), `moves` say who
// moved. After the end (not blind) the board is the usual one: the last move as it comes.
export function lastMoveToShow({ last, moves, you, blind, oppLast }) {
  if (!last || last.length < 2) return null;
  const lm = { from: last[0], to: last[last.length - 1] };
  if (!blind) return lm;
  if (!you) return lm;
  const side = moves?.length ? moves[moves.length - 1].side : "";
  return oppLast && side && side !== you ? lm : null;
}

// ------------------------------------------------------------------ settings

// The person's own choices in the room, kept in the browser: whether the list of moves is shown (it is
// not: the point is to hold the position in the head), whether the dots of a chosen piece are drawn and
// whether the opponent's last move is lit on the empty board (on).
export const DEFAULT_PREFS = { showMoves: false, dots: true, oppLast: true };
const KEY = "dyanis.blindfold.v1";

export function readPrefs(storage) {
  try {
    const raw = JSON.parse(storage?.getItem(KEY) || "null");
    return {
      showMoves: typeof raw?.showMoves === "boolean" ? raw.showMoves : DEFAULT_PREFS.showMoves,
      dots: typeof raw?.dots === "boolean" ? raw.dots : DEFAULT_PREFS.dots,
      oppLast: typeof raw?.oppLast === "boolean" ? raw.oppLast : DEFAULT_PREFS.oppLast,
    };
  } catch {
    return { ...DEFAULT_PREFS };
  }
}

export function writePrefs(storage, prefs) {
  try {
    storage?.setItem(KEY, JSON.stringify(prefs));
  } catch {
    /* private window, full storage: the choice lasts until the page is closed */
  }
}

// Peeking lasts this long.
export const PEEK_MS = 3000;

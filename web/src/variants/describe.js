// Words for the end of a game of a variant, and the list of moves. Pure, covered by `npm test`.
import { describeFinished } from "../play/game.js";

// How the games end, by the key the server sends in `termination`: [reason, "how" for the subtitle].
// A variant adds its own in `reasons` of its index.js.
export const BASE_REASONS = {
  checkmate: ["Мат", "матом"],
  stalemate: ["Пат", ""],
  resign: ["Сдача", "по сдаче соперника"],
  timeout: ["Время вышло", "по времени"],
  agreement: ["По соглашению", ""],
  repetition: ["Троекратное повторение", ""],
  "fifty-moves": ["Правило 50 ходов", ""],
  "insufficient-material": ["Недостаточно материала", ""],
  "king-captured": ["Король взят", "взятием короля"],
  "no-moves": ["Нет ходов", "из-за отсутствия ходов"],
};

// The description of a finished game for the summary card, see play/game.js describeFinished.
export function describeEnd({ result, termination, you, reasons }) {
  return describeFinished({ result, termination, you, reasons: { ...BASE_REASONS, ...reasons } });
}

// The moves of a game grouped into turns (consecutive actions of one side: a duck game has two, a dice
// game three): [{ side, items: [record] }].
export function groupTurns(moves) {
  const turns = [];
  for (const m of moves || []) {
    const last = turns[turns.length - 1];
    if (last && last.side === m.side) last.items.push(m);
    else turns.push({ side: m.side, items: [m] });
  }
  return turns;
}

// The turns as rows of the list: [[whiteTurn | null, blackTurn | null], ...].
export function listRows(turns) {
  const rows = [];
  for (let i = 0; i < turns.length; i++) {
    const t = turns[i];
    if (t.side === "white") {
      const next = turns[i + 1];
      if (next && next.side === "black") {
        rows.push([t, next]);
        i++;
      } else rows.push([t, null]);
    } else rows.push([null, t]);
  }
  return rows;
}

// A move action that the board can make by itself, from one square to another ("e2e4", "e7e8q"); the
// rest (drops "P@e4", the duck "@e5", the end of a turn "-") is the variant's.
export const BOARD_MOVE = /^[a-h][1-8][a-h][1-8][qrbn]?$/;
export const isBoardMove = (action) => BOARD_MOVE.test(action);

// Adds the class names of b to those of a for the same square: { e1: "a" } + { e1: "b" } -> { e1: "a b" }.
export function mergeClasses(a, b) {
  const out = { ...(a || {}) };
  for (const [sq, cls] of Object.entries(b || {})) out[sq] = out[sq] ? `${out[sq]} ${cls}` : cls;
  return out;
}

// The squares of the last action as the board's lastMove: two squares, or one (a drop), or none.
export function lastMoveOf(last) {
  if (!last || last.length === 0) return null;
  return { from: last[0], to: last[last.length - 1] };
}

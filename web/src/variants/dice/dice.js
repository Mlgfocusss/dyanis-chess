// The logic of the dice screen, apart from React so that `npm test` covers it.
// The server sends view.extra = { of: 3, dice: [{ die: "N", state: "free" | "wait" | "used" }], left, turns, lost? }:
// the three dice of the turn, thrown together when it began. "free": not used and a piece of the kind can move;
// "used": a move was made with it; "skip": no piece of the kind could move, skipped for good (it never comes back).
import { isBoardMove } from "../describe.js";

export const FACE = { P: 1, N: 2, B: 3, R: 4, Q: 5, K: 6 }; // the number on the die for a kind
export const NAME = { P: "пешка", N: "конь", B: "слон", R: "ладья", Q: "ферзь", K: "король" };
export const SIDE_NOM = { white: "Белые", black: "Чёрные" };

// Pip positions of a face on a 48 x 48 square.
const A = 14;
const B = 24;
const C = 34;
export const PIPS = {
  1: [[B, B]],
  2: [[A, A], [C, C]],
  3: [[A, A], [B, B], [C, C]],
  4: [[A, A], [C, A], [A, C], [C, C]],
  5: [[A, A], [C, A], [B, B], [A, C], [C, C]],
  6: [[A, A], [C, A], [A, B], [C, B], [A, C], [C, C]],
};

/** Squares of the pieces that may move now (any kind with an unused die), as board classes: { e2: "dice-movable" }. */
export function movableSquares(legal) {
  const out = {};
  for (const a of legal || []) if (isBoardMove(a)) out[a.slice(0, 2)] = "dice-movable";
  return out;
}

/** The three dice of the turn as the server sent them; always three slots (null when there are none). */
export function diceSlots(extra) {
  const dice = extra?.dice || [];
  return [0, 1, 2].map((i) => dice[i] || null);
}

/** The side whose colour the pieces of the dice wear: the one to move ("white" | "black"). */
export const sideOfDice = (turn) => (turn === "black" ? "black" : "white");

/** How many dice have been used in this turn. */
export function usedCount(extra) {
  return (extra?.dice || []).filter((d) => d.state === "used").length;
}

/**
 * Whether the person to move still has to press the die: the turn has begun, nothing is played yet and they have
 * not rolled (`rolled` is the page's own memory of the press; a reload forgets it, and the same dice come back).
 */
export function needsRoll(extra, myTurn, rolled) {
  return !!extra?.dice?.length && !!myTurn && !rolled && usedCount(extra) === 0;
}

/** The name of the icon of a kind ("N" -> "pieceN", drawn in ui/Icon.jsx). */
export const pieceIcon = (kind) => "piece" + kind;

/** "пешка, конь" for the dice that can still be played; "" when none. */
export function leftNames(extra) {
  return (extra?.dice || [])
    .filter((d) => d.state === "free")
    .map((d) => NAME[d.die])
    .join(", ");
}

/** The line about the dice for the person to move, and for everybody else. */
export function describeTurn(extra, myTurn, turn) {
  if (!extra?.dice?.length) return "";
  const left = extra.left ?? extra.dice.filter((d) => d.state === "free").length;
  if (left === 0) return "Все три кубика сыграны.";
  const names = leftNames(extra);
  return myTurn
    ? `Осталось ходов: ${left} из ${extra.of || 3}. Ходите любой из фигур в любом порядке: ${names}.`
    : `${SIDE_NOM[turn] || ""}: осталось ходов ${left} из ${extra.of || 3} (${names}).`;
}

/** The short caption under a die. */
export function dieCaption(state) {
  return { free: "можно ходить", skip: "нечем ходить, пропуск", used: "сыграно", empty: "" }[state] ?? "";
}

/** "Чёрные пропустили очередь: ..." for a turn that was lost whole, or "". */
export function describeLost(lost) {
  if (!lost) return "";
  return `${SIDE_NOM[lost.side]} пропустили очередь: выпали ${(lost.dice || []).map((d) => NAME[d]).join(", ")}, ходить было нечем.`;
}

/**
 * Whether the dice of this view are a new throw the page has not shown yet. seen is the turn counter of the throw
 * last shown (undefined before the first view, which only remembers: a reload does not play the animation again).
 * Returns { seen, fresh }.
 */
export function newThrow(seen, extra) {
  const key = extra?.dice?.length ? extra.turns : undefined;
  if (key === undefined) return { seen, fresh: false };
  if (seen === undefined) return { seen: key, fresh: false };
  return { seen: key, fresh: key !== seen };
}

/** Three random kinds for the shaking of the dice (rand: () => [0, 1)). */
export function shakeKinds(rand = Math.random) {
  const kinds = Object.keys(FACE);
  return [0, 1, 2].map(() => kinds[Math.floor(rand() * kinds.length)]);
}

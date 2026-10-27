import { tr } from "../../i18n/index.js";
// King of the Hill: the four central squares and how far each king is from them. Pure (covered by `npm test`).

export const HILL = ["d4", "e4", "d5", "e5"];

/** The css classes for the squares of the hill. */
export function hillClasses() {
  return Object.fromEntries(HILL.map((sq) => [sq, "sq-hill"]));
}

const fileOf = (sq) => sq.charCodeAt(0) - 97;
const rankOf = (sq) => Number(sq[1]) - 1;

/** The king's moves to the nearest square of the hill on an empty board (0 on the hill), or null with no king. */
export function hillDistance(board, side) {
  const code = side === "white" ? "wK" : "bK";
  const at = Object.keys(board || {}).find((sq) => board[sq] === code);
  if (!at) return null;
  return Math.min(...HILL.map((h) => Math.max(Math.abs(fileOf(h) - fileOf(at)), Math.abs(rankOf(h) - rankOf(at)))));
}

/** "на горе", "1 ход", "3 хода", "5 ходов". */
export function describeDistance(n) {
  if (n === null || n === undefined) return "";
  if (n === 0) return tr("variants.onHill");
  return tr("variants.hillMoves", { n });
}

import { tr } from "../../i18n/index.js";
// Horde: how many pieces the white horde still has. Pure (covered by `npm test`).

/** The size of the horde: the server's count in view.extra.horde, or counted from the board. */
export function hordeSize(view) {
  const n = view && view.extra && view.extra.horde;
  if (Number.isInteger(n)) return n;
  return Object.values((view && view.board) || {}).filter((code) => code[0] === "w").length;
}

/** "36 фигур", "1 фигура", "2 фигуры". */
export function describeHorde(n) {
  return tr("variants.hordePieces", { n });
}

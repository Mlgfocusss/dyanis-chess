// Horde: how many pieces the white horde still has. Pure (covered by `npm test`).

/** The size of the horde: the server's count in view.extra.horde, or counted from the board. */
export function hordeSize(view) {
  const n = view && view.extra && view.extra.horde;
  if (Number.isInteger(n)) return n;
  return Object.values((view && view.board) || {}).filter((code) => code[0] === "w").length;
}

/** "36 фигур", "1 фигура", "2 фигуры". */
export function describeHorde(n) {
  const last = n % 10;
  const teen = n % 100 >= 11 && n % 100 <= 14;
  const word = teen || last === 0 || last >= 5 ? "фигур" : last === 1 ? "фигура" : "фигуры";
  return `${n} ${word}`;
}

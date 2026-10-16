// Pure logic of the promotion choice, with no imports, so `npm test` covers it.

// How the pieces are offered, as on chess.com: queen first.
export const PROMOTION_ORDER = ["q", "n", "r", "b"];

// The promotion letters ("q", "n", ...) of the legal moves from `from` to `to`, in the order
// they are offered; null when the move is no promotion (one candidate without a letter).
// `moves` are the legal moves of the board: [{ uci }].
export function promotionLetters(moves, from, to) {
  const letters = moves
    .filter((m) => m.uci.length === 5 && m.uci.slice(0, 2) === from && m.uci.slice(2, 4) === to)
    .map((m) => m.uci[4]);
  if (letters.length === 0) return null;
  return PROMOTION_ORDER.filter((l) => letters.includes(l));
}

// "w" or "b": whose pawn stands on `square` of the FEN ("" when no piece does).
export function pieceSide(fen, square) {
  const rows = (fen || "").split(" ")[0].split("/");
  const row = rows[8 - Number(square[1])];
  if (!row) return "";
  let file = 0;
  for (const c of row) {
    if (c >= "1" && c <= "8") file += Number(c);
    else if ("abcdefgh"[file++] === square[0]) return c === c.toUpperCase() ? "w" : "b";
  }
  return "";
}

// Whether the picker, which opens on the promotion square and runs inwards, runs upwards on the screen:
// it does when the square is on the bottom edge as the board is shown.
export function pickerRunsUp(square, orientation) {
  return (orientation === "white") === (square[1] === "1");
}

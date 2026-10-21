// Atomic chess: the last explosion as the server tells it (view.extra.blast), turned into what the screen shows.
// Pure (covered by `npm test`). The server sends { center: "d4", squares: ["d4", "c5", ...], pieces: { d4: "bP", ... } }:
// the squares where a piece stood and is no more (not the one the capturing piece left), with the pieces that were there.

const NAMES = {
  P: ["пешка", "белая", "чёрная"],
  N: ["конь", "белый", "чёрный"],
  B: ["слон", "белый", "чёрный"],
  R: ["ладья", "белая", "чёрная"],
  Q: ["ферзь", "белый", "чёрный"],
  K: ["король", "белый", "чёрный"],
};

/** The blast of a view's extra, or null when the last action blew nothing up. */
export function blastOf(extra) {
  const b = extra && extra.blast;
  if (!b || !Array.isArray(b.squares) || b.squares.length === 0) return null;
  return { center: b.center || "", squares: b.squares, pieces: b.pieces || {} };
}

/** The css classes to put on the board's squares: the whole blast tinted, the centre stronger. */
export function blastClasses(blast) {
  if (!blast) return undefined;
  const out = {};
  for (const sq of blast.squares) out[sq] = sq === blast.center ? "sq-blast sq-blast--center" : "sq-blast";
  return out;
}

/** "белый конь" for a piece code ("wN"), "" for something unknown. */
export function pieceName(code) {
  const n = NAMES[code && code[1]];
  if (!n || (code[0] !== "w" && code[0] !== "b")) return "";
  return `${code[0] === "w" ? n[1] : n[2]} ${n[0]}`;
}

/** What was lost, for the card: [{ square, name, side }] (the centre first, then by square) and the count for each side. */
export function describeBlast(blast) {
  if (!blast) return null;
  const lost = blast.squares
    .map((square) => ({ square, name: pieceName(blast.pieces[square]), side: blast.pieces[square] && blast.pieces[square][0] }))
    .filter((l) => l.name)
    .sort((a, b) => (a.square === blast.center ? -1 : b.square === blast.center ? 1 : a.square.localeCompare(b.square)));
  return {
    center: blast.center,
    lost,
    white: lost.filter((l) => l.side === "w").length,
    black: lost.filter((l) => l.side === "b").length,
  };
}

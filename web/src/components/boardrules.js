// Pure rules the board needs besides the legal moves it is given, with no imports, so `npm test`
// covers them: the way a castling is made, a preview of a move waiting for its confirmation, and
// the moves that may be made in advance (premoves).

const FILES = "abcdefgh";

// The pieces of a FEN: { "e4": "P", ... } (white upper case, black lower).
export function piecesOf(fen) {
  const out = {};
  const rows = (fen || "").split(" ")[0].split("/");
  rows.forEach((row, i) => {
    let file = 0;
    for (const c of row) {
      if (c >= "1" && c <= "8") file += Number(c);
      else out["abcdefgh"[file++] + (8 - i)] = c;
    }
  });
  return out;
}

// The FEN placement of a piece map.
function placementOf(pieces) {
  const rows = [];
  for (let rank = 8; rank >= 1; rank--) {
    let row = "";
    let empty = 0;
    for (const f of FILES) {
      const p = pieces[f + rank];
      if (p) {
        if (empty) row += empty;
        empty = 0;
        row += p;
      } else empty++;
    }
    if (empty) row += empty;
    rows.push(row);
  }
  return rows.join("/");
}

// ------------------------------------------------------------------ castling

// The rook of a castling made by `uci` ("e1g1": the king of the first rank two squares to the side):
// { rook: "h1" (where the rook stands), to: "f1" } or null when the move is no castling.
export function castlingOf(fen, uci) {
  const from = uci.slice(0, 2);
  const to = uci.slice(2, 4);
  const p = piecesOf(fen)[from];
  if (p !== "K" && p !== "k") return null;
  const rank = p === "K" ? "1" : "8";
  if (from !== "e" + rank || to[1] !== rank) return null;
  if (to[0] === "g") return { rook: "h" + rank, to: "f" + rank };
  if (to[0] === "c") return { rook: "a" + rank, to: "d" + rank };
  return null;
}

// The square a person clicks or drops the king on to make the castling `uci`, as the way of castling
// ("standard": the king two squares, "rook": the king onto its rook, "both") allows. null: not that way.
export function castlingSquares(fen, uci, mode) {
  const c = castlingOf(fen, uci);
  if (!c) return null;
  const squares = [];
  if (mode !== "rook") squares.push(uci.slice(2, 4));
  if (mode !== "standard") squares.push(c.rook);
  return squares;
}

// The legal move (uci) a person means by taking the piece on `from` to `to`, or null. A castling
// follows the way chosen in the settings; every other move is as given.
export function resolveMove(legal, fen, from, to, mode = "standard") {
  for (const m of legal) {
    if (m.uci.slice(0, 2) !== from) continue;
    const squares = castlingSquares(fen, m.uci, mode);
    if (squares) {
      if (squares.includes(to)) return m.uci;
    } else if (m.uci.slice(2, 4) === to) {
      return m.uci;
    }
  }
  return null;
}

// The squares the piece on `from` may be taken to, as the person sees them (a castling shows the
// squares of the chosen way).
export function targetSquares(legal, fen, from, mode = "standard") {
  const out = new Set();
  for (const m of legal) {
    if (m.uci.slice(0, 2) !== from) continue;
    const squares = castlingSquares(fen, m.uci, mode);
    if (squares) squares.forEach((s) => out.add(s));
    else out.add(m.uci.slice(2, 4));
  }
  return out;
}

// ------------------------------------------------------------------- preview

// The placement of the FEN after the move `uci`, for showing a move that waits for confirmation:
// a castling moves the rook, en passant takes the pawn, a promotion changes the piece.
export function previewPlacement(fen, uci) {
  const pieces = piecesOf(fen);
  const from = uci.slice(0, 2);
  const to = uci.slice(2, 4);
  let p = pieces[from];
  if (!p) return placementOf(pieces);
  const castle = castlingOf(fen, uci);
  if (castle) {
    pieces[castle.to] = pieces[castle.rook];
    delete pieces[castle.rook];
  }
  if ((p === "P" || p === "p") && from[0] !== to[0] && !pieces[to]) {
    // en passant: the opposing pawn beside the mover goes (nothing else is ever taken this way)
    const beside = pieces[to[0] + from[1]];
    if (beside && (beside === "P" || beside === "p") && beside !== p) delete pieces[to[0] + from[1]];
  }
  if (uci.length === 5) p = p === "P" ? uci[4].toUpperCase() : uci[4].toLowerCase();
  delete pieces[from];
  pieces[to] = p;
  return placementOf(pieces);
}

// ---------------------------------------------------------------- premoves

// The position as it would be if the premoves of the queue were all played one after another
// (a FEN-like string: the pieces are right, the rest is not meant to be read). The next premove is
// chosen on this position.
export function queuedPosition(fen, queue) {
  let current = fen;
  for (const uci of queue || []) current = previewPlacement(current, uci) + " w - - 0 1";
  return current;
}

const KNIGHT = [[1, 2], [2, 1], [2, -1], [1, -2], [-1, -2], [-2, -1], [-2, 1], [-1, 2]];
const KING = [[1, 0], [1, 1], [0, 1], [-1, 1], [-1, 0], [-1, -1], [0, -1], [1, -1]];
const ROOK = [[1, 0], [0, 1], [-1, 0], [0, -1]];
const BISHOP = [[1, 1], [-1, 1], [-1, -1], [1, -1]];

const isWhite = (p) => p === p.toUpperCase();

// The squares the piece on `from` could go to if the position were different enough: the geometry of
// its moves, blocked only by the person's own pieces (the opponent's may move away) and with no regard
// for checks. A pawn reaching the last rank becomes a queen. Returns UCI strings.
export function premoveMoves(fen, from) {
  const pieces = piecesOf(fen);
  const p = pieces[from];
  if (!p) return [];
  const white = isWhite(p);
  const file = FILES.indexOf(from[0]);
  const rank = Number(from[1]);
  const out = [];
  const own = (sq) => pieces[sq] && isWhite(pieces[sq]) === white;
  const at = (f, r) => (f >= 0 && f < 8 && r >= 1 && r <= 8 ? FILES[f] + r : null);
  const add = (sq, promo = "") => out.push(from + sq + promo);
  const kind = p.toLowerCase();

  if (kind === "p") {
    const dir = white ? 1 : -1;
    const last = white ? 8 : 1;
    const promo = (r) => (r === last ? "q" : "");
    const one = at(file, rank + dir);
    if (one && !pieces[one]) {
      add(one, promo(rank + dir));
      const two = at(file, rank + 2 * dir);
      if (rank === (white ? 2 : 7) && two && !pieces[two]) add(two);
    }
    for (const df of [-1, 1]) {
      const sq = at(file + df, rank + dir);
      if (sq && !own(sq)) add(sq, promo(rank + dir));
    }
  } else if (kind === "n" || kind === "k") {
    for (const [df, dr] of kind === "n" ? KNIGHT : KING) {
      const sq = at(file + df, rank + dr);
      if (sq && !own(sq)) add(sq);
    }
    if (kind === "k" && from === (white ? "e1" : "e8")) {
      const r = white ? 1 : 8;
      const rook = white ? "R" : "r";
      if (pieces["h" + r] === rook && !own("f" + r) && !own("g" + r)) add("g" + r);
      if (pieces["a" + r] === rook && !own("b" + r) && !own("c" + r) && !own("d" + r)) add("c" + r);
    }
  } else {
    const dirs = kind === "r" ? ROOK : kind === "b" ? BISHOP : [...ROOK, ...BISHOP];
    for (const [df, dr] of dirs) {
      let f = file + df;
      let r = rank + dr;
      for (let sq = at(f, r); sq; f += df, r += dr, sq = at(f, r)) {
        if (own(sq)) break;
        add(sq);
      }
    }
  }
  return out;
}

// All the premoves of the pieces of `side` ("w" or "b"): [{ uci }], the shape the board takes legal moves in.
export function premoveList(fen, side) {
  const out = [];
  for (const [sq, p] of Object.entries(piecesOf(fen))) {
    if ((side === "w") === isWhite(p)) for (const uci of premoveMoves(fen, sq)) out.push({ uci });
  }
  return out;
}

// The move to play for a premove once it is the person's turn: the premove itself when it is legal
// (a promotion with the queen), else null.
export function premoveToPlay(legal, premove) {
  if (!premove) return null;
  const ucis = legal.map((m) => (typeof m === "string" ? m : m.uci));
  if (ucis.includes(premove)) return premove;
  if (premove.length === 4 && ucis.includes(premove + "q")) return premove + "q";
  return null;
}

// "w" or "b" for the colour a game orientation or a side name stands for.
export const colourLetter = (side) => (side === "black" || side === "b" ? "b" : "w");

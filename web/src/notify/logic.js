// The pure part of the notice about a move in a correspondence game: which games are new, how the tab
// title looks. Plain JavaScript with no imports, so it is covered by `npm test`.

// "(2) Dyanis Chess" for two games waiting for a move, the plain title for none.
export function titleWithCount(title, n) {
  const base = String(title || "").replace(/^\(\d+\)\s+/, "");
  return n > 0 ? `(${n}) ${base}` : base;
}

// The count of games waiting for the person went up: something new to announce. `prev` is null before
// the first answer of the server (a reload must not announce what was already waiting).
export function countWentUp(prev, next) {
  return prev !== null && prev !== undefined && next > prev;
}

// A game of the list "my correspondence games" that is a new call for a move: it is the person's turn
// in it and either the person has not been told about it, or it has had moves since (the person moved
// and the opponent answered). `known` is a Map of game id to the number of half-moves it had when the
// person was last told. Returns { fresh, known } with the new Map: every game with the person to move.
export function freshTurns(list, known) {
  const mine = (list || []).filter((g) => g.yourTurn);
  const fresh = mine.filter((g) => !known.has(g.id) || g.plies > known.get(g.id));
  return { fresh, known: new Map(mine.map((g) => [g.id, g.plies])) };
}

// The id of the game whose room is open in the address (`#online/<id>`), or "".
export function openRoomId(hash) {
  const m = /^#online\/([^/?#]+)/.exec(hash || "");
  return m ? m[1] : "";
}

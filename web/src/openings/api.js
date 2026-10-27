import { serverMsg, tr } from "../i18n/index.js";
// Client of the repertoire API (backend/internal/repertoire).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON (a 204, or an error page): handled below
  }
  if (!res.ok) {
    const e = new Error(serverMsg(body?.error) || tr("common.serverReplied", { status: res.status }));
    e.status = res.status;
    throw e;
  }
  return body;
}

const JSON_HEADERS = { "Content-Type": "application/json" };
const base = (id) => `/api/repertoires/${encodeURIComponent(id)}`;

// [{id, name, color, counts: {moves, mine, new, due, learned}}]
export async function listRepertoires() {
  return (await readJSON(await fetch("/api/repertoires"))).repertoires;
}

export async function createRepertoire({ name, color }) {
  return readJSON(await fetch("/api/repertoires", { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ name, color }) }));
}

// { repertoire, moves: [{id, parent, uci, san, fen, ply, mine, box, due, lapses, reviews}], gaps: [id] }
export async function getRepertoire(id) {
  return readJSON(await fetch(base(id)));
}

export async function renameRepertoire(id, name) {
  return readJSON(await fetch(base(id), { method: "PATCH", headers: JSON_HEADERS, body: JSON.stringify({ name }) }));
}

export async function deleteRepertoire(id) {
  const res = await fetch(base(id), { method: "DELETE" });
  if (!res.ok) await readJSON(res);
}

// A repertoire as a PGN file (the lines as variations): a link the browser downloads.
export const exportUrl = (id) => `${base(id)}/export`;

// A new repertoire from PGN text; color "" takes the Orientation tag of the file.
// { repertoire: {id, name, color}, import: {games, skipped, added, moves} }
export async function importRepertoire({ name, color, pgn }) {
  return readJSON(await fetch("/api/repertoires/import", { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ name, color, pgn }) }));
}

// PGN text added to a repertoire: { import: {games, skipped, added, moves} }
export async function importInto(id, pgn) {
  return readJSON(await fetch(`${base(id)}/import`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ pgn }) }));
}

// { move, created }
export async function addMove(id, parent, uci) {
  return readJSON(await fetch(`${base(id)}/moves`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ parent, uci }) }));
}

// { deleted: n }: the move and every line after it
export async function deleteMove(id, move) {
  return readJSON(await fetch(`${base(id)}/moves/${move}`, { method: "DELETE" }));
}

// { color, name, drills: [...], dueTotal, newTotal, nextDue }
export async function fetchSession(id, newCount = 10) {
  return readJSON(await fetch(`${base(id)}/train?new=${newCount}`));
}

// { box, due, lapses, learned }
export async function sendReview(id, move, correct) {
  return readJSON(await fetch(`${base(id)}/review`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ move, correct }) }));
}

// The moves of a position that opening theory (the ECO reference) knows, the
// best known first: { book, moves: [{uci, san, opening?: {eco, name}, lines}] }.
export async function fetchContinuations(fen) {
  return readJSON(await fetch("/api/continuations", { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ fen }) }));
}

// Statistics of the Lichess opening explorer for a position (through our server, which
// holds the token): { available, total, white, draws, black, moves: [{uci, san, games,
// white, draws, black, averageRating, opening?}] }. ratings is a comma-separated list of
// the explorer's rating groups ("1400,1600"). Throws with status 429 when Lichess asks to wait.
export async function fetchExplorer(fen, ratings) {
  const q = new URLSearchParams({ fen, ratings });
  return readJSON(await fetch(`/api/explorer?${q}`));
}

// The person's recent games put against a repository: { games: [{id, opponent, bot, result,
// gameStatus, endedMs, deviation: {status, ply, node, line, played, expected, moves}}], counts }.
export async function fetchGames(id) {
  return readJSON(await fetch(`${base(id)}/games`));
}

// Games the person pasted (PGN text) put against a repository, the same comparison as
// fetchGames: { games, counts, read, skipped, warning }. name is the person's name in the
// games' tags ("" takes every game as theirs).
export async function compareGames(id, pgn, name) {
  return readJSON(await fetch(`${base(id)}/compare`, { method: "POST", headers: JSON_HEADERS, body: JSON.stringify({ pgn, name }) }));
}

// The search of the repository for holes (popular replies of the opponent it has no answer to):
// { gaps: [{kind: "missing"|"answer", at, ply, fen, path, uci, san, pct, games, lines, weight, source}],
//   coverage (0..1), counts, partial, pending, source ("lichess"|"theory"|"mixed"), available,
//   limited, retryAfter, ratings, minPct, at }. It asks Lichess for up to 40 positions that the
// server has not got in its cache, so it takes a moment, and a partial answer is completed by asking again.
export async function fetchGaps(id, { ratings, minPct } = {}) {
  const q = new URLSearchParams();
  if (ratings) q.set("ratings", ratings);
  if (minPct) q.set("minPct", String(minPct));
  return readJSON(await fetch(`${base(id)}/gaps?${q}`));
}

// The last saved search at once, nobody is asked; null when there was none. `stale` is set when
// the tree has changed since.
export async function fetchSavedGaps(id) {
  const res = await fetch(`${base(id)}/gaps?saved=1`);
  if (res.status === 204) return null;
  return readJSON(res);
}

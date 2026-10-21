// Client of the variants' game API (backend/internal/vplay).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    const e = new Error(body?.error || `Сервер ответил ${res.status}`);
    e.status = res.status;
    throw e;
  }
  return body;
}

const post = (url, body) =>
  fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

// Makes a game; options are the lobby's choices of the variant (undefined for a variant with none).
export async function createGame({ variant, color, timeMs, incMs, options }) {
  const res = await post("/api/variants/games", { variant, color, timeMs, incMs, ...(options ? { options } : {}) });
  return (await readJSON(res)).id;
}

// The review of a finished game (only Chess960 has one): the id of the review, opened at #review/<id>. Asking again
// for the same game gives the same id.
export async function createReview(gameId) {
  const res = await post("/api/analyses/variant", { game: gameId });
  return (await readJSON(res)).id;
}

// { game, you }: the same message the socket sends.
export async function getGame(id) {
  return readJSON(await fetch(`/api/variants/games/${encodeURIComponent(id)}`));
}

// seat: the place to take in a game of four (Bughouse), 0..3; without it the first free one.
export async function joinGame(id, seat) {
  const init = { method: "POST" };
  if (seat !== undefined) Object.assign(init, { headers: { "Content-Type": "application/json" }, body: JSON.stringify({ seat }) });
  return readJSON(await fetch(`/api/variants/games/${encodeURIComponent(id)}/join`, init));
}

export async function listGames() {
  return (await readJSON(await fetch("/api/variants/games"))).items;
}

export const socketUrl = (id) =>
  `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/api/variants/games/${encodeURIComponent(id)}/ws`;

// The address to send a friend.
export const inviteUrl = (id) => `${window.location.origin}/#variants/game/${id}`;

// For useGame (online/useGame.js): where to read a game and open its socket. One object for good.
export const GAME_API = { getGame, socketUrl };

// The refusal to start a game while another is on, when the person has asked for one game at a time
// (settings → Игровой процесс). Resolves when a game may be started; rejects with the reason.
export async function ensureNoGameRunning(settings) {
  if (settings.multiGames) return;
  let items = [];
  try {
    items = await listGames();
  } catch {
    return; // not signed in yet, or the list is unavailable: nothing to check
  }
  if (items.some((g) => g.status === "playing")) {
    throw new Error("У вас уже идёт партия: закончите её или разрешите несколько партий в настройках («Игровой процесс»).");
  }
}

// The quick game: { variant, baseMs, incMs, options } -> { status: "none" | "waiting" | "found", id, waited, count }.
export async function seekMatch(req) {
  return readJSON(await post("/api/variants/seek", req));
}

// Asking also tells the server that the person is still waiting.
export async function checkSeek() {
  return readJSON(await fetch("/api/variants/seek"));
}

// Leaves the queue. If a game had just been made, the answer is { status: "found", id }.
export async function cancelSeek() {
  return readJSON(await fetch("/api/variants/seek", { method: "DELETE" }));
}

// For useSeek (online/useSeek.js). One object for good.
export const SEEK_API = { seekMatch, checkSeek, cancelSeek };

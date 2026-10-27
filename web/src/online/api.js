import { serverMsg, tr } from "../i18n/index.js";
// Client of the online-game API (backend/internal/play).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    const e = new Error(serverMsg(body?.error) || tr("common.serverReplied", { status: res.status }));
    e.status = res.status;
    throw e;
  }
  return body;
}

export async function createGame({ color, timeMs, incMs, rated = false, days = 0 }) {
  const res = await fetch("/api/games", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ color, timeMs, incMs, rated, days }),
  });
  return (await readJSON(res)).id;
}

// { game, you }: the same message the socket sends.
export async function getGame(id) {
  return readJSON(await fetch(`/api/games/${encodeURIComponent(id)}`));
}

export async function joinGame(id) {
  return readJSON(await fetch(`/api/games/${encodeURIComponent(id)}/join`, { method: "POST" }));
}

// PGN text of a finished game, to send to the review.
export async function fetchPgn(id) {
  const res = await fetch(`/api/games/${encodeURIComponent(id)}/pgn`);
  if (!res.ok) await readJSON(res);
  return res.text();
}

export async function listGames() {
  return (await readJSON(await fetch("/api/games"))).items;
}

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
  // a correspondence game lasts for days: it does not count as "a game that is on"
  const going = items.find((g) => g.status === "playing" && !g.tc?.days);
  if (going) {
    throw new Error(tr("onlineApi.2"));
  }
}

// The rating limits of a search from the settings: how much weaker and stronger an opponent may be
// (0 = no limit).
export const seekLimits = (settings) => ({ rangeDown: settings.seekDown || 0, rangeUp: settings.seekUp || 0 });

export const socketUrl = (id) =>
  `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/api/games/${encodeURIComponent(id)}/ws`;

// The address to send a friend.
export const inviteUrl = (id) => `${window.location.origin}/#online/${id}`;

// ------------------------------------------------------------ matchmaking and ratings

// Asks for a game with a person: { kind: "chess", baseMs, incMs, rated } or { kind: "duel" }.
// Answers with the state of the search, see checkSeek.
export async function seekMatch(req) {
  const res = await fetch("/api/match/seek", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  return readJSON(res);
}

// { status: "none" | "waiting" | "found", kind, id (the game or the duel), waited, window, count }.
// Asking also tells the server that the person is still waiting.
export async function checkSeek() {
  return readJSON(await fetch("/api/match/seek"));
}

// Leaves the queue. If a game had just been made, the answer is { status: "found", id }.
export async function cancelSeek() {
  return readJSON(await fetch("/api/match/seek", { method: "DELETE" }));
}

// [{category, name, rating, rd, provisional, games, wins, draws, losses}] for the signed-in person.
export async function fetchRatings() {
  return (await readJSON(await fetch("/api/ratings"))).ratings;
}

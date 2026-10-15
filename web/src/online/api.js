// Client of the online-game API (backend/internal/play).

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

export async function createGame({ color, timeMs, incMs }) {
  const res = await fetch("/api/games", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ color, timeMs, incMs }),
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

export const socketUrl = (id) =>
  `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/api/games/${encodeURIComponent(id)}/ws`;

// The address to send a friend.
export const inviteUrl = (id) => `${window.location.origin}/#online/${id}`;

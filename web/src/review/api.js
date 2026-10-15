// Client of the review API (internal/api).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    throw new Error(body?.error || `Сервер ответил ${res.status}`);
  }
  return body;
}

export async function createAnalysis({ pgn, game = 1, depth = 0 }) {
  let res;
  try {
    res = await fetch("/api/analyses", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pgn, game, depth }),
    });
  } catch {
    throw new Error("Не удалось связаться с сервером разбора. Запущен ли cmd/server?");
  }
  return (await readJSON(res)).id;
}

export async function getAnalysis(id) {
  return readJSON(await fetch(`/api/analyses/${encodeURIComponent(id)}`));
}

// The signed-in user's reviews, newest first (rows of api.Listed).
export async function listAnalyses() {
  return (await readJSON(await fetch("/api/analyses"))).items;
}

export async function deleteAnalysis(id) {
  const res = await fetch(`/api/analyses/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!res.ok) await readJSON(res); // throws with the server's message
}

// Plays moves (UCI or SAN) from a position on the server, which knows the rules:
// { steps: [{uci, san, fen}], fen, legal: [uci], status, eval? }. With eval the
// engine also judges the last position (depth 0 = server default).
export async function fetchLine({ fen, moves = [], eval: wantEval = false, depth = 0 }) {
  const res = await fetch("/api/line", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fen, moves, eval: wantEval, depth }),
  });
  return readJSON(res);
}

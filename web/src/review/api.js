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

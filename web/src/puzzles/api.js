// Client of the puzzle API (backend/internal/puzzles).

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

// { puzzle: {id, rating, themes, fen, orientation, steps}, you?: {rating, attempts, solved} }
// theme: "" for any; id: a particular puzzle instead of the next one.
export async function fetchPuzzle({ theme = "", id = "" } = {}) {
  const url = id ? `/api/puzzles/${encodeURIComponent(id)}` : `/api/puzzles/next${theme ? `?theme=${encodeURIComponent(theme)}` : ""}`;
  return readJSON(await fetch(url));
}

// [{id, count}], the most common theme first.
export async function fetchThemes() {
  return (await readJSON(await fetch("/api/puzzles/themes"))).themes;
}

// The outcome of the first try: { counted, before, after, you: {rating, attempts, solved} }.
export async function sendResult(id, solved) {
  const res = await fetch(`/api/puzzles/${encodeURIComponent(id)}/result`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ solved }),
  });
  return readJSON(res);
}

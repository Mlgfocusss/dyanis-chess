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
// theme: "" for any; id: a particular puzzle instead of the next one; daily: the puzzle of the day
// (the answer then also has daily: {date, status}, status "new" | "solved" | "failed").
export async function fetchPuzzle({ theme = "", id = "", daily = false, level = "" } = {}) {
  if (daily) return readJSON(await fetch("/api/puzzles/daily"));
  const q = new URLSearchParams();
  if (theme) q.set("theme", theme);
  if (level) q.set("level", level); // "random" | "easier" | "normal" | "harder": settings → Игровой процесс
  const url = id ? `/api/puzzles/${encodeURIComponent(id)}` : `/api/puzzles/next${q.size ? `?${q}` : ""}`;
  return readJSON(await fetch(url));
}

// [{id, count}], the most common theme first.
export async function fetchThemes() {
  return (await readJSON(await fetch("/api/puzzles/themes"))).themes;
}

// The first try at the puzzle of the day (practice, the rating stays): { date, status: "solved" | "failed" }.
export async function sendDailyResult(solved) {
  const res = await fetch("/api/puzzles/daily/result", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ solved }),
  });
  return readJSON(res);
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

// A puzzle for the tactics storm near a rating, none of `exclude` (ids already met in the run).
// Same shape as fetchPuzzle, with no `you`.
export async function fetchRushPuzzle(rating, exclude = []) {
  const q = new URLSearchParams({ rating: String(rating), exclude: exclude.join(",") });
  return readJSON(await fetch(`/api/puzzles/rush/next?${q}`));
}

// The person's records by mode: { "3min": { best, runs } }.
export async function fetchRushBest() {
  return (await readJSON(await fetch("/api/puzzles/rush/best"))).best;
}

// A finished run; answers with the records after it, as fetchRushBest does.
export async function sendRushResult(mode, score) {
  const res = await fetch("/api/puzzles/rush/result", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode, score }),
  });
  return (await readJSON(res)).best;
}

// ------------------------------------------------------------------ puzzle duel

// The duel as the person sees it: { id, status: "countdown" | "running" | "over", serverNow, startsAt,
// endsAt (unix ms), me, opponent: {name, avatar, rating, provisional, score, mistakes, index, change?},
// outcome?: "win" | "loss" | "draw" }.
export async function fetchDuel(id) {
  return readJSON(await fetch(`/api/duels/${encodeURIComponent(id)}`));
}

// Puzzle number k of the duel: { puzzle } as fetchPuzzle gives; the same for both people.
export async function fetchDuelPuzzle(id, k) {
  return readJSON(await fetch(`/api/duels/${encodeURIComponent(id)}/puzzle/${k}`));
}

// The end of the puzzle number `index`: solved or not. Answers with the duel, as fetchDuel does; a
// report out of step is answered with status 409 and the duel (e.error is set, the body is lost).
export async function sendDuelProgress(id, index, solved) {
  const res = await fetch(`/api/duels/${encodeURIComponent(id)}/progress`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ index, solved }),
  });
  return readJSON(res);
}

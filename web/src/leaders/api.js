import { tr, serverMsg } from "../i18n/index.js";
// Client of the public leaderboard and profile API (backend/internal/profiles).

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

// The best players of a category (of one country when a code is given): [{ place, id, kind, name, avatarUrl,
// country, rating, games }].
export async function fetchLeaderboard(category, country, signal) {
  const filter = country ? `&country=${encodeURIComponent(country)}` : "";
  const res = await fetch(`/api/leaderboard?category=${encodeURIComponent(category)}${filter}&limit=50`, { signal });
  return (await readJSON(res)).rows || [];
}

// The public profile of a player; an error with status 404 for an unknown one.
export async function fetchUser(id, signal) {
  return readJSON(await fetch(`/api/users/${encodeURIComponent(id)}`, { signal }));
}

// Client of the achievements API (backend/internal/achievements).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    const e = new Error(body?.error || `HTTP ${res.status}`);
    e.status = res.status;
    throw e;
  }
  return body;
}

// The badges of the signed-in person with progress: { items, unlocked, total, unseen }.
// poll: the quiet check of the notice in the corner, which the server may answer from a minute-old count.
export async function fetchMine({ poll = false, signal } = {}) {
  return readJSON(await fetch(poll ? "/api/achievements?poll=1" : "/api/achievements", { signal }));
}

// Says that the notices of these badges were shown (no ids: all of them).
export async function markSeen(ids = []) {
  return readJSON(
    await fetch("/api/achievements/seen", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids }),
    }),
  );
}

// The badges another person has earned: { items: [{ id, category, tier, unlockedAt }], unlocked, total }.
export async function fetchPublic(userId, signal) {
  return readJSON(await fetch(`/api/users/${encodeURIComponent(userId)}/achievements`, { signal }));
}

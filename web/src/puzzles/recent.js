// The last puzzles the person finished, kept in this browser (all kinds: rated, by theme, of the day), so
// that they can be opened again and looked at. Pure logic with the storage given in, so `npm test` covers it.
export const RECENT_KEY = "dyanis.puzzleRecent";
export const RECENT_MAX = 10;

const MODES = ["rated", "themes", "daily"];

// An entry: { id, mode, ok (solved at the first try), rating (of the puzzle), at (ms) }.
function clean(e) {
  if (!e || typeof e.id !== "string" || !e.id || e.id.length > 40) return null;
  return {
    id: e.id,
    mode: MODES.includes(e.mode) ? e.mode : "rated",
    ok: !!e.ok,
    rating: Number.isFinite(e.rating) ? Math.round(e.rating) : 0,
    at: Number.isFinite(e.at) ? e.at : 0,
  };
}

// The list with the entry first; the same puzzle is not listed twice; at most `max`.
export function addRecent(list, entry, max = RECENT_MAX) {
  const e = clean(entry);
  if (!e) return list;
  return [e, ...list.filter((x) => x.id !== e.id)].slice(0, max);
}

export function readRecent(storage) {
  try {
    const raw = JSON.parse(storage.getItem(RECENT_KEY) || "[]");
    if (!Array.isArray(raw)) return [];
    return raw.map(clean).filter(Boolean).slice(0, RECENT_MAX);
  } catch {
    return [];
  }
}

export function saveRecent(storage, list) {
  try {
    storage.setItem(RECENT_KEY, JSON.stringify(list));
  } catch {
    // a private window: the list just lives until the page is closed
  }
}

// Pure helpers of the achievements pages: grouping, counters, the choice of notices.

// The order of the categories on the page (the server sends the badges in this order as well).
export const CATEGORY_ORDER = ["games", "bots", "rating", "puzzles", "review", "openings", "lessons", "community", "streaks"];
export const TIERS = ["bronze", "silver", "gold"];

// The symbol of each category in the badge (see AchievementBadge.jsx).
export const KNOWN_CATEGORIES = new Set(CATEGORY_ORDER);

// Items grouped by category in page order: [{ category, items, unlocked, total }]. A category the page
// does not know (added by the server later) goes to the end, in the order it came.
export function groupByCategory(items) {
  const groups = new Map(CATEGORY_ORDER.map((c) => [c, []]));
  for (const it of items || []) {
    if (!groups.has(it.category)) groups.set(it.category, []);
    groups.get(it.category).push(it);
  }
  return [...groups.entries()]
    .filter(([, list]) => list.length > 0)
    .map(([category, list]) => ({ category, items: list, unlocked: list.filter((i) => i.unlocked).length, total: list.length }));
}

// How many badges of each tier are earned and how many there are: { bronze: { n, total }, ... }.
export function tierTotals(items) {
  const out = Object.fromEntries(TIERS.map((t) => [t, { n: 0, total: 0 }]));
  for (const it of items || []) {
    const row = out[it.tier];
    if (!row) continue;
    row.total++;
    if (it.unlocked) row.n++;
  }
  return out;
}

// The width of the bar of a badge, 0..100. A badge that is earned is full; one without a bar has none.
export function progressPercent(it) {
  if (!it) return 0;
  if (it.unlocked) return 100;
  if (!(it.target > 1)) return 0;
  return Math.max(0, Math.min(100, Math.floor((100 * it.progress) / it.target)));
}

// A yes-or-no badge has no progress bar.
export function showsBar(it) {
  return !!it && it.target > 1;
}

// Share of the earned badges, a whole percent.
export function overallPercent(unlocked, total) {
  if (!(total > 0)) return 0;
  return Math.max(0, Math.min(100, Math.round((100 * unlocked) / total)));
}

// The newest earned badges of a public list (already newest first from the server), at most n.
export function latest(items, n = 6) {
  return [...(items || [])].sort((a, b) => String(b.unlockedAt).localeCompare(String(a.unlockedAt))).slice(0, n);
}

// At most this many notices are drawn one by one; a longer list (a person who has just got the
// badges of everything they ever did) becomes one notice with the number.
export const MAX_SINGLE_TOASTS = 3;

// What to show for the unseen ids: [{ key, id }] one by one, or [{ key, many: n }].
export function planToasts(unseen, shown = new Set()) {
  const fresh = (unseen || []).filter((id) => !shown.has(id));
  if (fresh.length === 0) return [];
  if (fresh.length > MAX_SINGLE_TOASTS) return [{ key: `many:${fresh[0]}`, many: fresh.length, ids: fresh }];
  return fresh.map((id) => ({ key: id, id, ids: [id] }));
}

// Seconds between two asks of the server that the notice makes (a minute, or sooner after a page change).
export const POLL_MS = 60_000;
export const MIN_GAP_MS = 15_000;
export const TOAST_MS = 9_000;

// May the notice ask the server now? `last` is the time of the previous ask (0: never).
export function mayAsk(now, last, gap = MIN_GAP_MS) {
  return now - last >= gap;
}

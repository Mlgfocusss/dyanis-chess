import { tr } from "../i18n/index.js";
// Words and categories for ratings, with no imports, so `npm test` covers them.

export const CATEGORY_NAMES = { bullet: "Пуля", blitz: "Блиц", rapid: "Рапид", duel: "Дуэль" };

// The name of a rating category in the language of the site ("Пуля" / "Bullet"); "" for an unknown one.
export function categoryName(category) {
  return CATEGORY_NAMES[category] ? tr("rating.cat." + category) : "";
}

// The category of a game by its clock, as the server counts it (ratings.CategoryFor): the base time
// plus 40 moves of increment, under 3 minutes is bullet, under 8 blitz, the rest rapid.
export function categoryOf(minutes, incSeconds) {
  const total = minutes * 60 + 40 * incSeconds;
  if (total < 180) return "bullet";
  if (total < 480) return "blitz";
  return "rapid";
}

// "1500" or "1500?" for a rating that is still a rough guess.
export function ratingText(rating, provisional) {
  return `${rating}${provisional ? "?" : ""}`;
}

// "+15" / "−8" / "±0".
export function deltaText(before, after) {
  const d = after - before;
  return d > 0 ? `+${d}` : d < 0 ? `−${-d}` : "±0";
}

// The line under a finished rated game: "Блиц: 1200 → 1215 (+15)"; "" without a change.
export function changeLine(change) {
  if (!change) return "";
  const name = categoryName(change.category) || tr("rating.rating");
  return `${name}: ${change.before} → ${ratingText(change.after, change.provisional)} (${deltaText(change.before, change.after)})`;
}

// How a search for a game is told: "12 с · ищем соперника с разницей до 275".
export function searchLine(status) {
  if (!status) return "";
  const parts = [tr("rating.seconds", { n: status.waited || 0 })];
  if (status.window) parts.push(tr("rating.window", { w: Math.round(status.window) }));
  if (status.count > 1) parts.push(tr("rating.queue", { n: status.count }));
  return parts.join(" · ");
}

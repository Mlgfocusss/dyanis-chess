// Words and categories for ratings, with no imports, so `npm test` covers them.

export const CATEGORY_NAMES = { bullet: "Пуля", blitz: "Блиц", rapid: "Рапид", duel: "Дуэль" };

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
  const name = CATEGORY_NAMES[change.category] || "Рейтинг";
  return `${name}: ${change.before} → ${ratingText(change.after, change.provisional)} (${deltaText(change.before, change.after)})`;
}

// How a search for a game is told: "12 с · ищем соперника с разницей до 275".
export function searchLine(status) {
  if (!status) return "";
  const parts = [`${status.waited || 0} с`];
  if (status.window) parts.push(`разница в рейтинге до ${Math.round(status.window)}`);
  if (status.count > 1) parts.push(`в очереди: ${status.count}`);
  return parts.join(" · ");
}

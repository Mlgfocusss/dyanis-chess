// Pure logic of the tactics storm ("Тактический штурм"), with no imports, so `npm test` covers it.

// The lengths of a run, as the server names them.
export const RUSH_MODES = {
  "3min": { label: "3 минуты", ms: 3 * 60 * 1000 },
  "5min": { label: "5 минут", ms: 5 * 60 * 1000 },
};

// The rating the next puzzle should have after `solved` puzzles: easy at first, harder with each one.
export function targetRating(solved) {
  return Math.min(2800, 500 + 50 * Math.max(0, solved));
}

// "2:59" for the time left, rounded up so that the clock shows 0:00 only when time is out.
export function clockText(ms) {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

// How a finished run is told: { headline, record }.
// reason is "time", "mistake" or "quit"; best is the best score before this run.
export function runSummary(reason, score, best) {
  const record = score > 0 && score > best;
  const word = reason === "mistake" ? "Ошибка" : reason === "quit" ? "Серия окончена" : "Время вышло";
  return { headline: `${word}: решено ${score}`, record };
}

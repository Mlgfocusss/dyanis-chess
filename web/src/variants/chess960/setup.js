// Chess960 helpers without React, so `npm test` covers them. The server draws the setup number and tells it
// in `extra` ({ number, rank }); this reads what the person types in the lobby and words the setup.

export const COUNT = 960;
export const CLASSIC = 518;

// The number a lobby text means: an integer 0..959 as a number, "" as null (a random setup), else NaN.
export function parseNumber(text) {
  const t = String(text ?? "").trim();
  if (t === "") return null;
  if (!/^\d{1,3}$/.test(t)) return NaN;
  const n = Number(t);
  return n < COUNT ? n : NaN;
}

// The pieces of a first rank by their Russian letters, a-file first: "RNBQKBNR" -> ["Л", "К", "С", "Ф", "Кр", "С", "К", "Л"].
const NAMES = { R: "Л", N: "К", B: "С", Q: "Ф", K: "Кр" };
export function rankLetters(rank) {
  return [...(rank || "")].map((c) => NAMES[c] || c);
}

// "Расстановка 518 (обычная)" / "Расстановка 12".
export function describeSetup(number) {
  return number === CLASSIC ? `Расстановка ${number} (обычная)` : `Расстановка ${number}`;
}

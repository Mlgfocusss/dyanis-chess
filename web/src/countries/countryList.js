// Pure helpers for the list of countries (GET /api/countries: [{ code, ru, en }], sorted by the Russian
// name by the server, the only source of the list). No React here, so `npm test` covers it.

// Text for comparing: lower case, "ё" as "е", typographic apostrophes as the plain one, runs of spaces folded.
export function foldText(s) {
  return String(s ?? "")
    .toLocaleLowerCase("ru")
    .replace(/ё/g, "е")
    .replace(/[’`ʼ]/g, "'")
    .replace(/\s+/g, " ")
    .trim();
}

// How well a country matches a (folded) query: 0 is the best, -1 means no match.
//   0 the Russian name starts with it   1 a word of it starts with it   2 it is inside the name
//   3 the English name has it           4 it is the country's code
function score(country, q) {
  const ru = foldText(country.ru);
  if (ru.startsWith(q)) return 0;
  if (ru.split(/[\s-]+/).some((w) => w.startsWith(q))) return 1;
  if (ru.includes(q)) return 2;
  if (foldText(country.en).includes(q)) return 3;
  if (q.length === 2 && country.code.toLowerCase() === q) return 4;
  return -1;
}

// The countries that match what was typed, the best match first and, among equals, in the order of the
// list. An empty query gives the whole list.
export function searchCountries(list, query) {
  const all = Array.isArray(list) ? list : [];
  const q = foldText(query);
  if (!q) return all;
  const found = [];
  all.forEach((c, i) => {
    const s = score(c, q);
    if (s >= 0) found.push({ c, s, i });
  });
  found.sort((a, b) => a.s - b.s || a.i - b.i);
  return found.map((x) => x.c);
}

// The Russian name of a code; the code itself when the list does not know it (or is not loaded yet);
// "" for no country.
export function countryName(list, code) {
  if (!code) return "";
  const c = (Array.isArray(list) ? list : []).find((x) => x.code === code);
  return c ? c.ru : code;
}

// "Беларусь", "Минск, Беларусь" or "": the place line under a name from a location and a country.
export function placeLine(location, countryNameText) {
  return [String(location ?? "").trim(), String(countryNameText ?? "").trim()].filter(Boolean).join(", ");
}

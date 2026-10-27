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

// How well a country matches a (folded) query: 0 is the best, -1 means no match. `lang` is the language
// of the site: its name of the country is the main one, the other language's name still finds it.
//   0 the main name starts with it   1 a word of it starts with it   2 it is inside the name
//   3 the other name has it          4 it is the country's code
function score(country, q, lang) {
  const main = foldText(lang === "en" ? country.en || country.ru : country.ru);
  const other = foldText(lang === "en" ? country.ru : country.en);
  if (main.startsWith(q)) return 0;
  if (main.split(/[\s-]+/).some((w) => w.startsWith(q))) return 1;
  if (main.includes(q)) return 2;
  if (other.includes(q)) return 3;
  if (q.length === 2 && country.code.toLowerCase() === q) return 4;
  return -1;
}

// The countries that match what was typed, the best match first and, among equals, in the order of the
// list. An empty query gives the whole list.
export function searchCountries(list, query, lang = "ru") {
  const all = Array.isArray(list) ? list : [];
  const q = foldText(query);
  if (!q) return all;
  const found = [];
  all.forEach((c, i) => {
    const s = score(c, q, lang);
    if (s >= 0) found.push({ c, s, i });
  });
  found.sort((a, b) => a.s - b.s || a.i - b.i);
  return found.map((x) => x.c);
}

// The name of a code in the language (Russian by default); the code itself when the list does not know it
// (or is not loaded yet); "" for no country.
export function countryName(list, code, lang = "ru") {
  if (!code) return "";
  const c = (Array.isArray(list) ? list : []).find((x) => x.code === code);
  return c ? (lang === "en" ? c.en || c.ru : c.ru) : code;
}

// The list in the order of the language: the server sends it sorted by the Russian name, English is sorted here.
export function sortCountries(list, lang = "ru") {
  const all = Array.isArray(list) ? list : [];
  if (lang !== "en") return all;
  return [...all].sort((a, b) => String(a.en || a.ru).localeCompare(String(b.en || b.ru), "en"));
}

// "Беларусь", "Минск, Беларусь" or "": the place line under a name from a location and a country.
export function placeLine(location, countryNameText) {
  return [String(location ?? "").trim(), String(countryNameText ?? "").trim()].filter(Boolean).join(", ");
}

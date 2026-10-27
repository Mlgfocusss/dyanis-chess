// Pure logic of the leaderboard and public profile pages, with no imports from React, so `npm test` covers it.
import { countryName } from "../countries/countryList.js";
import { categoryName as catName, ratingText } from "../online/rating.js";
import { getLanguage, locale, tr } from "../i18n/index.js";

// Categories in the order of the tabs.
export const CATEGORIES = ["bullet", "blitz", "rapid", "duel"];
export const DEFAULT_CATEGORY = "blitz";

// Games a person needs for the table (the server counts the same: profiles.MinGames).
export const MIN_GAMES = 5;

export function categoryName(category) {
  return catName(category) || tr("rating.rating");
}

// A country code as the server takes it: two capital Latin letters ("" for all countries).
export function isCountryCode(code) {
  return typeof code === "string" && /^[A-Z]{2}$/.test(code);
}

// Addresses: "#leaders/<category>[/<country>]", "#user/<id>" (and "#user/<id>/<category>[/<country>]"
// remembers which table to go back to).
export function leadersHash(category, country = "") {
  if (!isCountryCode(country)) return CATEGORIES.includes(category) ? `#leaders/${category}` : "#leaders";
  return `#leaders/${CATEGORIES.includes(category) ? category : DEFAULT_CATEGORY}/${country}`;
}

export function userHash(id, category, country = "") {
  const base = `#user/${encodeURIComponent(id)}`;
  if (!CATEGORIES.includes(category)) return base;
  return isCountryCode(country) ? `${base}/${category}/${country}` : `${base}/${category}`;
}

// The country of "#leaders/<category>/<country>"; "" (all countries) for anything else.
export function countryFromHash(hash) {
  const [page, , country] = String(hash || "").replace(/^#/, "").split("/");
  return page === "leaders" && isCountryCode(country) ? country : "";
}

// The category of "#leaders/<category>"; the default for anything else.
export function categoryFromHash(hash) {
  const [page, category] = String(hash || "").replace(/^#/, "").split("/");
  return page === "leaders" && CATEGORIES.includes(category) ? category : DEFAULT_CATEGORY;
}

// { id, from, country } of "#user/<id>[/<category>[/<country>]]"; id is "" when the address names nobody.
export function userFromHash(hash) {
  const [page, id, from, country] = String(hash || "").replace(/^#/, "").split("/");
  if (page !== "user" || !id) return { id: "", from: "", country: "" };
  let decoded = id;
  try {
    decoded = decodeURIComponent(id);
  } catch {
    // keep the raw text: the server will say it is unknown
  }
  const ok = CATEGORIES.includes(from);
  return { id: decoded, from: ok ? from : "", country: ok && isCountryCode(country) ? country : "" };
}

// The rows best first: by rating, then by games played (more is firmer), then by name; the places are
// counted again from 1. The server sends them sorted already; this keeps the page right if it did not.
export function rankRows(rows) {
  const sorted = [...(rows || [])].sort(
    (a, b) => b.rating - a.rating || b.games - a.games || String(a.name).localeCompare(String(b.name), locale()),
  );
  return sorted.map((r, i) => ({ ...r, place: i + 1 }));
}

// Is the row the signed-in person's?
export function isMine(row, user) {
  return !!user && !!row && row.id === user.id;
}

// "1", "2", ...; "—" for a person with no place.
export function placeText(place) {
  return place > 0 ? String(place) : "—";
}

// "3-е место" for a profile; "" when there is no place.
export function placeLine(place) {
  return place > 0 ? tr("leaders.2", { place }) : "";
}

// 1, 2, 3 get a medal colour in the table: "gold", "silver", "bronze", otherwise "".
export function medal(place) {
  return place === 1 ? "gold" : place === 2 ? "silver" : place === 3 ? "bronze" : "";
}

// "Гость" or "Google": how the person signed in.
export function kindLabel(kind) {
  return kind === "google" ? "Google" : tr("leaders.3");
}

// "12 октября 2026", or "" for a date that is missing or silly.
export function joinedText(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  return d.toLocaleDateString(locale(), { day: "numeric", month: "long", year: "numeric" });
}

// "12 / 3 / 5": wins, draws, losses.
export function recordText(r) {
  return `${r.wins || 0} / ${r.draws || 0} / ${r.losses || 0}`;
}

// The ratings of a profile in the order of the tabs, each with the words the page shows. A category the
// server did not send is still there, as one with no games.
export function profileRatings(profile) {
  const by = new Map((profile?.ratings || []).map((r) => [r.category, r]));
  return CATEGORIES.map((category) => {
    const r = by.get(category) || { category, rating: 1200, games: 0 };
    const games = r.games || 0;
    return {
      category,
      name: categoryName(category),
      games,
      value: games ? ratingText(r.rating, r.provisional) : "—",
      record: games ? tr("leaders.4", { games, p: recordText(r) }) : tr("leaders.5"),
      place: r.place || 0,
      placeLine: placeLine(r.place || 0),
    };
  });
}

// "Побеждено 3 из 10 ботов".
export function botsLine(bots) {
  if (!bots || !bots.total) return "";
  return tr("leaders.6", { p: bots.beaten || 0, total: bots.total });
}

// Words under an empty table.
export function emptyLine(category) {
  return category === "duel"
    ? tr("leaders.7")
    : tr("leaders.8");
}

// Where "play" leads from the table of a category.
export function playHash(category) {
  return category === "duel" ? "#puzzles/duel" : "#online";
}

// The country column of the table: the Russian name from the loaded list, the code while the list is not
// there (or does not know it), an em dash for a person who named none.
export function countryCell(list, code) {
  if (!code) return "—";
  return countryName(list, code, getLanguage());
}

// The line over the table: "Страна: Беларусь" or "Все страны".
export function countryFilterLine(list, code) {
  return code ? tr("leaders.9", { p: countryName(list, code, getLanguage()) }) : tr("countries.all");
}

// The words under an empty table of one country.
export function emptyCountryLine(list, code) {
  return tr("leaders.11", { p: countryName(list, code, getLanguage()) });
}

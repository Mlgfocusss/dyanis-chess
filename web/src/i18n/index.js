// A small home-made i18n (no dependencies). Russian is the source language: every key exists in
// ru, and a key missing from en shows the Russian text (in the development build a warning is
// printed once per key). Keys are flat and grouped by area: "menu.home", "review.class.blunder".
//
// A value is a string or, for plurals, an object with the Intl.PluralRules categories:
//   "moves.count": { one: "{n} ход", few: "{n} хода", many: "{n} ходов", other: "{n} хода" }
//   t("moves.count", { n: 5 })      // the category is chosen by params.n (or params.count)
// "{name}" in a string is replaced by params.name.
//
// In a component: `const t = useT();` (the component draws again when the language changes).
// Outside a component (pure helpers that are called while drawing): `tr(key, params)`, which reads
// the language that is current at the moment; the component that calls the helper must itself
// use `useT()` so that it is redrawn on a change of language.
import { createContext, useContext } from "react";
import ru from "./ru.js";
import en from "./en.js";
import { LANGUAGES, DEFAULT_LANGUAGE, detectLanguage, isLanguage } from "./detect.js";

export { LANGUAGES, DEFAULT_LANGUAGE, detectLanguage, isLanguage };
const DICTS = { ru, en };
const LOCALES = { ru: "ru-RU", en: "en-US" };

let current = DEFAULT_LANGUAGE;
export function getLanguage() {
  return current;
}
export function setLanguage(lang) {
  if (isLanguage(lang)) current = lang;
}

// "ru-RU" / "en-US": for Intl and toLocale*String.
export function locale(lang = current) {
  return LOCALES[lang] || LOCALES.ru;
}

export function pluralCategory(lang, n) {
  try {
    return new Intl.PluralRules(locale(lang)).select(Number(n));
  } catch {
    return "other";
  }
}

// The dictionary entry of a key: the language's own, else the Russian one.
function lookup(lang, key) {
  const own = DICTS[lang] && DICTS[lang][key];
  if (own !== undefined) return own;
  if (lang !== DEFAULT_LANGUAGE) warnMissing(lang, key);
  return DICTS.ru[key];
}

const warned = new Set();
function warnMissing(lang, key) {
  const id = lang + ":" + key;
  if (warned.has(id)) return;
  warned.add(id);
  let dev = false;
  try {
    dev = !!(import.meta.env && import.meta.env.DEV);
  } catch {
    dev = false;
  }
  if (dev && typeof console !== "undefined") console.warn(`[i18n] no "${lang}" text for "${key}"`);
}

export function format(template, params) {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (m, name) => (params[name] === undefined || params[name] === null ? m : String(params[name])));
}

// Chooses the form of a plural entry for the number n.
function pickForm(entry, lang, n) {
  const cat = pluralCategory(lang, n);
  if (entry[cat] !== undefined) return entry[cat];
  if (entry.other !== undefined) return entry.other;
  if (entry.many !== undefined) return entry.many;
  return Object.values(entry)[0] ?? "";
}

export function translate(lang, key, params) {
  const entry = lookup(lang, key);
  if (entry === undefined) return key;
  if (typeof entry === "string") return format(entry, params);
  // a plural entry; the Russian one is the fallback when the English one lacks the category
  const n = params ? (params.n !== undefined ? params.n : params.count) : undefined;
  const useLang = DICTS[lang] && DICTS[lang][key] !== undefined ? lang : DEFAULT_LANGUAGE;
  return format(String(pickForm(entry, useLang, n === undefined ? 0 : n)), params);
}

// For code that is not a component (see the top of the file).
export function tr(key, params) {
  return translate(current, key, params);
}

// ---------------------------------------------------------------- numbers and dates

export function formatNumber(n, opts, lang = current) {
  return new Intl.NumberFormat(locale(lang), opts).format(n);
}

// `d` is a Date, a timestamp or an ISO string.
export function formatDate(d, opts = { day: "numeric", month: "long", year: "numeric" }, lang = current) {
  return new Date(d).toLocaleDateString(locale(lang), opts);
}

export function formatTime(d, opts = { hour: "2-digit", minute: "2-digit" }, lang = current) {
  return new Date(d).toLocaleTimeString(locale(lang), opts);
}

export function formatDateTime(d, opts = { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" }, lang = current) {
  return new Date(d).toLocaleString(locale(lang), opts);
}

// ---------------------------------------------------------------- React

export const I18nContext = createContext({ lang: DEFAULT_LANGUAGE, t: (key, params) => translate(DEFAULT_LANGUAGE, key, params) });

// The translate function of the current language; the component is drawn again on a change.
export function useT() {
  return useContext(I18nContext).t;
}

export function useLang() {
  return useContext(I18nContext).lang;
}

// The text of a key, or `fallback` when no dictionary knows the key (a variant added later
// without entries keeps its own Russian name).
export function tOr(t, key, fallback) {
  const s = t(key);
  return s === key ? fallback : s;
}

// [value, key] pairs (a module-level list of choices) -> [value, text] pairs for <Segmented>.
export function tOpts(t, pairs) {
  return pairs.map(([v, key]) => [v, t(key)]);
}

// A message the server sent in Russian (error texts of the Go code): shown in English when the
// language is English and the text is known (dictionary keys "srv.*" hold the Russian originals);
// anything else is shown as it came.
let srvMap = null;
export function serverMsg(msg) {
  if (!msg || typeof msg !== "string" || current === "ru") return msg;
  if (!srvMap) {
    srvMap = new Map();
    for (const [k, v] of Object.entries(DICTS.ru)) if (k.startsWith("srv.") && typeof v === "string") srvMap.set(v, k);
  }
  const k = srvMap.get(msg);
  return k ? translate(current, k) : msg;
}

// Translates a text by its Russian original: for data that carries Russian strings of its own (the
// reasons a game ended, names in the definitions of the chess variants). The Russian text must be a
// value of the Russian dictionary; an unknown text, or the Russian language, gives it back unchanged.
let ruIndex = null;
export function tx(text) {
  if (!text || typeof text !== "string" || current === "ru") return text;
  if (!ruIndex) {
    ruIndex = new Map();
    for (const [k, v] of Object.entries(DICTS.ru)) if (typeof v === "string" && !ruIndex.has(v)) ruIndex.set(v, k);
  }
  const k = ruIndex.get(text);
  return k ? translate(current, k) : text;
}

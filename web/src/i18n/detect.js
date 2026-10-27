// The site's languages and the choice of the first one (no imports, so settings/model.js can use it).
export const LANGUAGES = ["ru", "en"];
export const DEFAULT_LANGUAGE = "ru";

export function isLanguage(v) {
  return v === "ru" || v === "en";
}

// ru* is Russian, anything else English. `nav` is navigator (or a stand-in in the tests); without
// a navigator (node) the answer is the source language.
export function detectLanguage(nav = globalThis.navigator) {
  if (!nav) return DEFAULT_LANGUAGE;
  try {
    const l = String(nav.language || (nav.languages && nav.languages[0]) || "");
    return l.toLowerCase().startsWith("ru") ? "ru" : "en";
  } catch {
    return DEFAULT_LANGUAGE;
  }
}

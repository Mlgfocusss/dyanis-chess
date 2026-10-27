import test from "node:test";
import assert from "node:assert/strict";
import ru from "./ru.js";
import en from "./en.js";
import { detectLanguage } from "./detect.js";
import { format, getLanguage, pluralCategory, serverMsg, setLanguage, tOr, translate, tr, tx } from "./index.js";
import { DEFAULTS, INTERFACE_KEYS, sanitize } from "../settings/model.js";

const placeholders = (v) => {
  const s = typeof v === "string" ? [v] : Object.values(v);
  return [...new Set(s.flatMap((x) => [...String(x).matchAll(/\{(\w+)\}/g)].map((m) => m[1])))].sort();
};

test("the English dictionary has exactly the keys of the Russian one", () => {
  const onlyRu = Object.keys(ru).filter((k) => !(k in en));
  const onlyEn = Object.keys(en).filter((k) => !(k in ru));
  assert.deepEqual(onlyRu, [], "keys without an English text");
  assert.deepEqual(onlyEn, [], "English keys the Russian dictionary does not know");
});

test("every text is a non-empty string or a plural entry, and both languages use the same {placeholders}", () => {
  for (const [key, v] of Object.entries(ru)) {
    const ok = typeof v === "string" ? v.length > 0 : v && typeof v === "object" && Object.keys(v).length > 0;
    assert.ok(ok, `ru ${key} is empty`);
    assert.equal(typeof v, typeof en[key], `${key}: a string in one language and a plural entry in the other`);
    assert.deepEqual(placeholders(en[key]), placeholders(v), `${key}: the placeholders differ`);
    if (typeof v === "object") {
      for (const form of Object.keys(v)) assert.ok(["zero", "one", "two", "few", "many", "other"].includes(form), `${key}: bad form ${form}`);
      assert.ok("other" in en[key] || "many" in en[key], `${key}: English needs "other"`);
    }
  }
});

test("substitution of {name}", () => {
  assert.equal(format("Hello, {name}!", { name: "Ann" }), "Hello, Ann!");
  assert.equal(format("{a} + {a} = {b}", { a: 1, b: 2 }), "1 + 1 = 2");
  assert.equal(format("keep {missing}", {}), "keep {missing}");
  assert.equal(format("no params", undefined), "no params");
  assert.equal(format("{n} зёрен", { n: 0 }), "0 зёрен");
});

test("plural categories follow the rules of each language", () => {
  const ruCat = (n) => pluralCategory("ru", n);
  assert.deepEqual([1, 2, 5, 11, 21, 22, 25, 0].map(ruCat), ["one", "few", "many", "many", "one", "few", "many", "many"]);
  const enCat = (n) => pluralCategory("en", n);
  assert.deepEqual([0, 1, 2, 5].map(enCat), ["other", "one", "other", "other"]);
});

test("translate picks the plural form of the language", () => {
  assert.equal(translate("ru", "common.days", { n: 1 }), "1 день");
  assert.equal(translate("ru", "common.days", { n: 3 }), "3 дня");
  assert.equal(translate("ru", "common.days", { n: 11 }), "11 дней");
  assert.equal(translate("ru", "common.days", { n: 21 }), "21 день");
  assert.equal(translate("en", "common.days", { n: 1 }), "1 day");
  assert.equal(translate("en", "common.days", { n: 3 }), "3 days");
});

test("a key missing from English shows the Russian text, an unknown key shows itself", () => {
  assert.equal(translate("en", "no.such.key"), "no.such.key");
  assert.equal(translate("ru", "menu.home"), "Главная");
  assert.equal(translate("en", "menu.home"), "Home");
  assert.equal(tOr((k) => translate("en", k), "no.such.key", "fallback"), "fallback");
});

test("tr follows the current language, tx and serverMsg translate texts by their Russian original", () => {
  setLanguage("ru");
  assert.equal(tr("menu.home"), "Главная");
  assert.equal(tx("Главная"), "Главная");
  setLanguage("en");
  try {
    assert.equal(getLanguage(), "en");
    assert.equal(tr("menu.home"), "Home");
    assert.equal(tx("Главная"), "Home");
    assert.equal(tx("Текст, которого нет в словаре"), "Текст, которого нет в словаре");
    assert.equal(serverMsg("текст сервера, которого нет"), "текст сервера, которого нет");
    assert.equal(tx(""), "");
  } finally {
    setLanguage("ru");
  }
  setLanguage("xx");
  assert.equal(getLanguage(), "ru", "an unknown language is ignored");
});

test("the language of the browser: ru* is Russian, anything else English", () => {
  assert.equal(detectLanguage({ language: "ru" }), "ru");
  assert.equal(detectLanguage({ language: "ru-RU" }), "ru");
  assert.equal(detectLanguage({ language: "RU-by" }), "ru");
  assert.equal(detectLanguage({ language: "en-US" }), "en");
  assert.equal(detectLanguage({ language: "be-BY" }), "en");
  assert.equal(detectLanguage({ languages: ["ru", "en"] }), "ru");
  assert.equal(detectLanguage({}), "en");
  assert.equal(detectLanguage(null), "ru"); // no navigator (node): the source language
});

test("the language setting is checked like the others", () => {
  assert.ok(["ru", "en"].includes(DEFAULTS.language));
  assert.ok(INTERFACE_KEYS.includes("language"));
  assert.equal(sanitize({ language: "en" }).language, "en");
  assert.equal(sanitize({ language: "ru" }).language, "ru");
  assert.equal(sanitize({ language: "de" }).language, DEFAULTS.language);
  assert.equal(sanitize({ language: 5 }).language, DEFAULTS.language);
  assert.equal(sanitize({}).language, DEFAULTS.language);
});

// The definitions of the chess variants carry Russian texts of their own (name, short, rules, reasons, hints);
// the screens show them through tx(), which finds the text in the Russian dictionary. A text that is not
// there would stay Russian in English.
test("every Russian text in the definitions of the variants is in the dictionary (so tx() can translate it)", async () => {
  const fs = await import("node:fs");
  const path = await import("node:path");
  const dir = path.join(path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1")), "..", "variants");
  const known = new Set(Object.values(ru).filter((v) => typeof v === "string"));
  const files = [path.join(dir, "describe.js")];
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (!e.isDirectory()) continue;
    for (const f of ["index.js", "index.jsx"]) if (fs.existsSync(path.join(dir, e.name, f))) files.push(path.join(dir, e.name, f));
  }
  const missing = [];
  for (const file of files) {
    const text = fs.readFileSync(file, "utf8")
      .split("\n")
      .filter((l) => !/^\s*(\/\/|\*|\/\*)/.test(l))
      .join("\n");
    for (const m of text.matchAll(/"((?:[^"\\n]|\.)*[А-Яа-яЁё](?:[^"\\n]|\.)*)"|`([^`$]*[А-Яа-яЁё][^`$]*)`/g)) {
      const s = (m[1] ?? m[2]).replace(/\\"/g, '"');
      if (!known.has(s)) missing.push(`${path.basename(path.dirname(file))}: ${s.slice(0, 60)}`);
    }
  }
  assert.deepEqual(missing, []);
});

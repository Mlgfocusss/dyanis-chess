import { test } from "node:test";
import assert from "node:assert/strict";
import { countryName, foldText, placeLine, searchCountries } from "./countryList.js";

const LIST = [
  { code: "AM", ru: "Армения", en: "Armenia" },
  { code: "BY", ru: "Беларусь", en: "Belarus" },
  { code: "DE", ru: "Германия", en: "Germany" },
  { code: "CI", ru: "Кот-д’Ивуар", en: "Côte d'Ivoire" },
  { code: "KZ", ru: "Казахстан", en: "Kazakhstan" },
  { code: "CD", ru: "Демократическая Республика Конго", en: "Democratic Republic of the Congo" },
  { code: "CG", ru: "Республика Конго", en: "Republic of the Congo" },
  { code: "PG", ru: "Папуа — Новая Гвинея", en: "Papua New Guinea" },
  { code: "NZ", ru: "Новая Зеландия", en: "New Zealand" },
  { code: "RU", ru: "Россия", en: "Russia" },
];

const codes = (list) => list.map((c) => c.code);

test("foldText", () => {
  assert.equal(foldText("  Ёлка   Е "), "елка е");
  assert.equal(foldText("Кот-д’Ивуар"), "кот-д'ивуар");
  assert.equal(foldText(null), "");
  assert.equal(foldText(undefined), "");
});

test("an empty query gives the whole list", () => {
  assert.equal(searchCountries(LIST, ""), LIST);
  assert.equal(searchCountries(LIST, "   "), LIST);
  assert.deepEqual(searchCountries(null, "бел"), []);
  assert.deepEqual(searchCountries(undefined, ""), []);
});

test("search by the Russian name, any case", () => {
  assert.deepEqual(codes(searchCountries(LIST, "бел")), ["BY"]);
  assert.deepEqual(codes(searchCountries(LIST, "БЕЛАРУСЬ")), ["BY"]);
  assert.deepEqual(codes(searchCountries(LIST, "  герм ")), ["DE"]);
  assert.deepEqual(codes(searchCountries(LIST, "xyz")), []);
});

test("prefix beats a word inside beats a match inside the name", () => {
  // "конго": two countries with the word in the middle, in list order
  assert.deepEqual(codes(searchCountries(LIST, "конго")), ["CD", "CG"]);
  // "новая": a word at the start of the second part of the name comes after the names that start with it
  assert.deepEqual(codes(searchCountries(LIST, "нов")), ["NZ", "PG"]);
  // "ермания" is inside "Германия" only
  assert.deepEqual(codes(searchCountries(LIST, "ермани")), ["DE"]);
  // "ка" starts "Казахстан" and is a word start nowhere else, but is inside "Республика Конго"
  assert.deepEqual(codes(searchCountries(LIST, "ка")), ["KZ", "CD", "CG"]);
});

test("ё is typed as е, apostrophes as typed", () => {
  assert.deepEqual(codes(searchCountries([{ code: "XX", ru: "Ёмкость", en: "Cap" }], "емк")), ["XX"]);
  assert.deepEqual(codes(searchCountries(LIST, "кот-д'")), ["CI"]);
  assert.deepEqual(codes(searchCountries(LIST, "кот-д’")), ["CI"]);
  assert.deepEqual(codes(searchCountries(LIST, "ивуар")), ["CI"]);
});

test("search by the English name and by the code", () => {
  assert.deepEqual(codes(searchCountries(LIST, "belarus")), ["BY"]);
  assert.deepEqual(codes(searchCountries(LIST, "Germany")), ["DE"]);
  assert.deepEqual(codes(searchCountries(LIST, "by")), ["BY"]);
  assert.deepEqual(codes(searchCountries(LIST, "KZ")), ["KZ"]);
  assert.deepEqual(codes(searchCountries(LIST, "cote")), []); // accents are not folded: type the Russian name
});

test("countryName", () => {
  assert.equal(countryName(LIST, "BY"), "Беларусь");
  assert.equal(countryName(LIST, "ZZ"), "ZZ"); // not in the list: the code shows rather than nothing
  assert.equal(countryName(LIST, ""), "");
  assert.equal(countryName(LIST, undefined), "");
  assert.equal(countryName([], "BY"), "BY"); // list not loaded yet
  assert.equal(countryName(null, "BY"), "BY");
});

test("placeLine", () => {
  assert.equal(placeLine("Минск", "Беларусь"), "Минск, Беларусь");
  assert.equal(placeLine("", "Беларусь"), "Беларусь");
  assert.equal(placeLine("Минск", ""), "Минск");
  assert.equal(placeLine("  ", undefined), "");
  assert.equal(placeLine(null, null), "");
});

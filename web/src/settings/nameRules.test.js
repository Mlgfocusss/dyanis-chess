import test from "node:test";
import assert from "node:assert/strict";
import { checkName, confirmsDelete, normalizeName, NAME_MAX } from "./nameRules.js";

test("names are trimmed and their spaces folded", () => {
  assert.equal(normalizeName("  Анна   Иванова "), "Анна Иванова");
  assert.equal(normalizeName("a  b"), "a b");
  assert.equal(normalizeName(null), "");
});

test("good names pass", () => {
  for (const n of ["Анна", "Ян", "Bob_the-2nd.", "  Анна  Иванова ", "Ñandú", "日本語", "я".repeat(NAME_MAX), "😀a".slice(2) + "b"]) {
    assert.equal(checkName(n).error, "", n);
  }
  assert.equal(checkName("  Анна   Иванова ").name, "Анна Иванова");
});

test("bad names say why", () => {
  for (const n of ["", "   ", "я", "я".repeat(NAME_MAX + 1)]) assert.match(checkName(n).error, /слишком/, JSON.stringify(n));
  for (const n of ["a\nb", "a\tb", "a\u0000b", "<b>Анна</b>", "Аня 🙂", "a/b", "a@b", 'a"b']) assert.match(checkName(n).error, /можно использовать/, JSON.stringify(n));
  assert.match(checkName("---").error, /хотя бы одна/);
  assert.match(checkName("...").error, /хотя бы одна/);
});

test("the length counts characters, not bytes", () => {
  assert.equal(checkName("я".repeat(24)).error, "");
  assert.notEqual(checkName("я".repeat(25)).error, "");
});

test("deleting is confirmed by the word in any case, or by the name", () => {
  assert.equal(confirmsDelete("УДАЛИТЬ", "УДАЛИТЬ", "Анна"), true);
  assert.equal(confirmsDelete(" удалить ", "УДАЛИТЬ", "Анна"), true);
  assert.equal(confirmsDelete("Анна", "УДАЛИТЬ", "Анна"), true);
  assert.equal(confirmsDelete("анна", "УДАЛИТЬ", "Анна"), false);
  assert.equal(confirmsDelete("", "УДАЛИТЬ", ""), false);
  assert.equal(confirmsDelete("удал", "УДАЛИТЬ", "Анна"), false);
});

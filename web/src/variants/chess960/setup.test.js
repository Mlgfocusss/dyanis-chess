import test from "node:test";
import assert from "node:assert/strict";
import { CLASSIC, describeSetup, parseNumber, rankLetters } from "./setup.js";

test("a setup number is read from the lobby text", () => {
  assert.equal(parseNumber(""), null);
  assert.equal(parseNumber("  "), null);
  assert.equal(parseNumber(undefined), null);
  assert.equal(parseNumber("0"), 0);
  assert.equal(parseNumber("518"), CLASSIC);
  assert.equal(parseNumber(" 959 "), 959);
  assert.equal(parseNumber("007"), 7);
  for (const bad of ["960", "1000", "-1", "5.5", "abc", "12a", "1e2"]) assert.ok(Number.isNaN(parseNumber(bad)), bad);
});

test("a first rank is shown with Russian piece letters", () => {
  assert.deepEqual(rankLetters("RNBQKBNR"), ["Л", "К", "С", "Ф", "Кр", "С", "К", "Л"]);
  assert.deepEqual(rankLetters(""), []);
  assert.deepEqual(rankLetters(undefined), []);
});

test("the setup is worded, the classical one says so", () => {
  assert.equal(describeSetup(518), "Расстановка 518 (обычная)");
  assert.equal(describeSetup(0), "Расстановка 0");
});

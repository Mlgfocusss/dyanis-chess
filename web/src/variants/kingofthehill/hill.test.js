import test from "node:test";
import assert from "node:assert/strict";
import { HILL, describeDistance, hillClasses, hillDistance } from "./hill.js";

test("the hill is the four central squares", () => {
  assert.deepEqual([...HILL].sort(), ["d4", "d5", "e4", "e5"]);
  assert.deepEqual(Object.keys(hillClasses()).sort(), ["d4", "d5", "e4", "e5"]);
  assert.ok(Object.values(hillClasses()).every((c) => c === "sq-hill"));
});

test("the distance of a king to the hill", () => {
  const board = { e1: "wK", e8: "bK", a1: "wR" };
  assert.equal(hillDistance(board, "white"), 3);
  assert.equal(hillDistance(board, "black"), 3);
  assert.equal(hillDistance({ e4: "wK", e8: "bK" }, "white"), 0);
  assert.equal(hillDistance({ c3: "wK", e8: "bK" }, "white"), 1);
  assert.equal(hillDistance({ a1: "wK" }, "white"), 3);
  assert.equal(hillDistance({ a1: "wK" }, "black"), null);
  assert.equal(hillDistance(undefined, "white"), null);
});

test("the distance in words, with the Russian plural", () => {
  assert.equal(describeDistance(0), "на горе");
  assert.equal(describeDistance(1), "1 ход");
  assert.equal(describeDistance(2), "2 хода");
  assert.equal(describeDistance(3), "3 хода");
  assert.equal(describeDistance(4), "4 хода");
  assert.equal(describeDistance(5), "5 ходов");
  assert.equal(describeDistance(null), "");
});

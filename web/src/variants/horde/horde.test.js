import test from "node:test";
import assert from "node:assert/strict";
import { describeHorde, hordeSize } from "./horde.js";

test("the horde is counted from the server's number, else from the board", () => {
  assert.equal(hordeSize({ extra: { horde: 36 }, board: {} }), 36);
  assert.equal(hordeSize({ board: { a1: "wP", b2: "wP", e8: "bK", d8: "bQ" } }), 2);
  assert.equal(hordeSize({ extra: { horde: 0 }, board: { a1: "wP" } }), 0);
  assert.equal(hordeSize(undefined), 0);
});

test("the size in words, with the Russian plural", () => {
  assert.equal(describeHorde(36), "36 фигур");
  assert.equal(describeHorde(21), "21 фигура");
  assert.equal(describeHorde(22), "22 фигуры");
  assert.equal(describeHorde(11), "11 фигур");
  assert.equal(describeHorde(1), "1 фигура");
  assert.equal(describeHorde(0), "0 фигур");
});

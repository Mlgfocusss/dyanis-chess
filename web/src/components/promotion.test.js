import { test } from "node:test";
import assert from "node:assert/strict";
import { pickerRunsUp, pieceSide, promotionLetters } from "./promotion.js";

const mv = (...u) => u.map((uci) => ({ uci }));

test("promotionLetters offers the pieces queen first", () => {
  const moves = mv("e7e8b", "e7e8r", "e7e8q", "e7e8n", "e7d8q", "a1a2");
  assert.deepEqual(promotionLetters(moves, "e7", "e8"), ["q", "n", "r", "b"]);
  assert.deepEqual(promotionLetters(moves, "e7", "d8"), ["q"]); // a capture that is not offered all four
});

test("promotionLetters is null for an ordinary move or an illegal pair", () => {
  const moves = mv("a1a2", "e7e8q");
  assert.equal(promotionLetters(moves, "a1", "a2"), null);
  assert.equal(promotionLetters(moves, "h1", "h2"), null);
  assert.equal(promotionLetters([], "e7", "e8"), null);
});

test("pieceSide reads the colour of the piece on a square", () => {
  const fen = "r3k3/4P3/8/8/8/8/8/4K2R w - - 0 1";
  assert.equal(pieceSide(fen, "e7"), "w");
  assert.equal(pieceSide(fen, "a8"), "b");
  assert.equal(pieceSide(fen, "h1"), "w");
  assert.equal(pieceSide(fen, "e5"), "");
});

test("the picker runs away from the edge the pawn is promoting at", () => {
  assert.equal(pickerRunsUp("e1", "white"), true); // black promotes at the bottom of a white-oriented board
  assert.equal(pickerRunsUp("e8", "white"), false);
  assert.equal(pickerRunsUp("e8", "black"), true);
  assert.equal(pickerRunsUp("e1", "black"), false);
});

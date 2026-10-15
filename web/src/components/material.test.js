import { test } from "node:test";
import assert from "node:assert/strict";
import { materialOf } from "./material.js";

const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

test("the start position is level with nothing taken", () => {
  const m = materialOf(START);
  assert.equal(m.diff, 0);
  assert.deepEqual(m.white, { q: 0, r: 0, b: 0, n: 0, p: 0 });
  assert.deepEqual(m.black, { q: 0, r: 0, b: 0, n: 0, p: 0 });
});

test("a pawn up for White after 1.e4 d5 2.exd5", () => {
  const m = materialOf("rnbqkbnr/ppp1pppp/8/3P4/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 2");
  assert.equal(m.diff, 1);
  assert.equal(m.white.p, 1); // White has taken a black pawn
  assert.equal(m.black.p, 0);
});

test("lead and captured pieces after trades", () => {
  // White has lost its b1 knight, Black its a8 rook: White is two points up
  const m = materialOf("1nbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/R1BQKBNR w KQk - 0 1");
  assert.equal(m.white.r, 1); // White took the rook
  assert.equal(m.white.n, 0);
  assert.equal(m.black.n, 1); // Black took the knight
  assert.equal(m.black.r, 0);
  assert.equal(m.diff, 2);
});

test("a promoted pawn is not a captured pawn", () => {
  // White has one pawn fewer and one queen more: the pawn became the queen on a3
  const m = materialOf("rnbqkbnr/pppppppp/8/8/8/Q7/PPPPPPP1/RNBQKBNR w KQkq - 0 1");
  assert.equal(m.black.p, 0, "no white pawn was captured");
  assert.equal(m.diff, 8); // +9 for the queen, -1 for the pawn
});

test("junk input does not throw", () => {
  assert.equal(materialOf("").diff, 0);
  assert.equal(materialOf(undefined).diff, 0);
});

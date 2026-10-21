import test from "node:test";
import assert from "node:assert/strict";
import { boardFromPlacement, fenFromBoard, piecesOf } from "./boardrules.js";

const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR";

test("an object position becomes the FEN placement of the same pieces", () => {
  const board = {};
  for (const [sq, p] of Object.entries(piecesOf(START))) board[sq] = (p === p.toUpperCase() ? "w" : "b") + p.toUpperCase();
  assert.equal(fenFromBoard(board), START);
  assert.equal(fenFromBoard({}), "8/8/8/8/8/8/8/8");
  assert.equal(fenFromBoard(undefined), "8/8/8/8/8/8/8/8");
});

test("and back again, the duck included", () => {
  const board = { e1: "wK", e8: "bK", c3: "xD", h4: "bQ" };
  const placement = fenFromBoard(board);
  assert.equal(placement, "4k3/8/8/8/7q/2x5/8/4K3");
  assert.deepEqual(boardFromPlacement(placement), board);
  assert.deepEqual(boardFromPlacement(`${placement} w - - 0 1`), board);
});

test("a position with many pawns of one side (the horde) survives the round trip", () => {
  const board = {};
  for (let r = 1; r <= 4; r++) for (const f of "abcdefgh") board[f + r] = "wP";
  board.e8 = "bK";
  assert.deepEqual(boardFromPlacement(fenFromBoard(board)), board);
});

import { test } from "node:test";
import assert from "node:assert/strict";
import { dayLabel, hintSquare, judgeMove, legalFor, pointsText, signed, squaresOf } from "./puzzle.js";

const puzzle = {
  steps: [
    { uci: "f2g3", san: "Bxg3", fen: "A" },
    { uci: "e6e7", san: "Re7", fen: "B", legal: ["e6e7", "e6e8", "h6h7"], alt: [{ uci: "h6h7", san: "Qh7#", fen: "M" }] },
    { uci: "b2b1", san: "Qb1+", fen: "C" },
    { uci: "b3c1", san: "Nc1", fen: "D", legal: ["b3c1", "b3a1"] },
  ],
};

test("the line's own move is right, and the last one finishes the puzzle", () => {
  assert.deepEqual(judgeMove(puzzle, 1, "e6e7"), { kind: "right", fen: "B", san: "Re7", done: false });
  assert.deepEqual(judgeMove(puzzle, 3, "b3c1"), { kind: "right", fen: "D", san: "Nc1", done: true });
});

test("another mating move solves it at once", () => {
  assert.deepEqual(judgeMove(puzzle, 1, "h6h7"), { kind: "mate", fen: "M", san: "Qh7#", done: true });
});

test("any other move is wrong", () => {
  assert.equal(judgeMove(puzzle, 1, "e6e8").kind, "wrong");
  assert.equal(judgeMove(puzzle, 9, "e6e7").kind, "wrong");
});

test("legal moves and hints come from the step", () => {
  assert.deepEqual(legalFor(puzzle, 3), [{ uci: "b3c1" }, { uci: "b3a1" }]);
  assert.deepEqual(legalFor(puzzle, 0), []);
  assert.equal(hintSquare(puzzle, 1), "e6");
  assert.equal(hintSquare(puzzle, 9), null);
});

test("helpers", () => {
  assert.deepEqual(squaresOf("e2e4"), { from: "e2", to: "e4" });
  assert.equal(squaresOf(""), null);
  assert.equal(signed(18), "+18");
  assert.equal(signed(-12), "−12");
  assert.equal(signed(0), "0");
});

test("dayLabel names the day in Russian", () => {
  assert.equal(dayLabel("2026-10-16"), "16 октября");
  assert.equal(dayLabel("2026-01-01"), "1 января");
  assert.equal(dayLabel("nonsense"), "");
});

test("points are named in Russian by the number", () => {
  assert.equal(pointsText(0), "0 очков");
  assert.equal(pointsText(1), "1 очко");
  assert.equal(pointsText(3), "3 очка");
  assert.equal(pointsText(11), "11 очков");
  assert.equal(pointsText(21), "21 очко");
  assert.equal(pointsText(112), "112 очков");
});

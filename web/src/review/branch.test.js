import { test } from "node:test";
import assert from "node:assert/strict";
import { classOfLoss, judgeBranchMove, lineText, moverLoss, numberedSans, sideOf } from "./branch.js";

test("classes follow the analysis thresholds", () => {
  const got = [0, 1.9, 2, 4.9, 5, 9.9, 10, 19.9, 20, 80].map(classOfLoss);
  assert.deepEqual(got, ["excellent", "excellent", "good", "good", "inaccuracy", "inaccuracy", "mistake", "mistake", "blunder", "blunder"]);
});

test("the loss is the mover's, whichever colour moves", () => {
  const before = { winPct: 60 };
  const after = { winPct: 45 };
  assert.equal(moverLoss(before, after, "white"), 15); // White's chance fell by 15
  assert.equal(moverLoss(before, after, "black"), 0); // Black gained
  assert.equal(moverLoss({ winPct: 40 }, { winPct: 52 }, "black"), 12);
  assert.equal(moverLoss(null, after, "white"), null);
  assert.equal(moverLoss(before, {}, "white"), null);
});

test("the engine's own move is best, others are judged by the loss", () => {
  const before = { winPct: 50, bestUci: "e2e4" };
  assert.deepEqual(judgeBranchMove({ uci: "e2e4" }, before, { winPct: 49 }, "white"), { cls: "best", loss: 0 });
  assert.deepEqual(judgeBranchMove({ uci: "a2a3" }, before, { winPct: 35 }, "white"), { cls: "mistake", loss: 15 });
  assert.equal(judgeBranchMove({ uci: "a2a3" }, before, undefined, "white"), null);
});

test("who moves", () => {
  assert.equal(sideOf("8/8/8/8/8/8/8/8 b - - 0 1"), "black");
  assert.equal(sideOf("8/8/8/8/8/8/8/8 w - - 0 1"), "white");
});

test("moves are numbered as in a score", () => {
  const steps = [{ san: "e4" }, { san: "e5" }, { san: "Nf3" }];
  const start = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";
  assert.equal(lineText(numberedSans(start, steps)), "1. e4 e5 2. Nf3");
  // a branch that starts with Black's move at move 7
  const mid = "r1bqkbnr/pppp1ppp/2n5/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 2 7";
  assert.equal(lineText(numberedSans(mid, [{ san: "Nf6" }, { san: "Nc3" }, { san: "Bb4" }])), "7... Nf6 8. Nc3 Bb4");
});

import test from "node:test";
import assert from "node:assert/strict";
import {
  chartMarks,
  decodeFen,
  encodeFen,
  errorCount,
  fenFromHash,
  looksLikeFen,
  momentPlies,
  moveLabel,
  reviewUrl,
  stepMoment,
  summaryRows,
} from "./moments.js";

const moves = [
  { ply: 1, color: "white", san: "e4", class: "book" },
  { ply: 2, color: "black", san: "e5", class: "book" },
  { ply: 3, color: "white", san: "Qh5", class: "inaccuracy" },
  { ply: 4, color: "black", san: "Nc6", class: "best" },
  { ply: 5, color: "white", san: "Bc4", class: "mistake" },
  { ply: 6, color: "black", san: "g6", class: "blunder" },
  { ply: 7, color: "white", san: "Qf3", class: "miss" },
  { ply: 8, color: "black", san: "Nf6", class: "brilliant" },
];

test("momentPlies takes the moves that cost something, in game order", () => {
  assert.deepEqual(momentPlies(moves), [5, 6, 7]);
  assert.deepEqual(momentPlies(moves, { side: "black" }), [6]);
  assert.deepEqual(momentPlies(moves, { kinds: ["inaccuracy"] }), [3]);
  assert.deepEqual(momentPlies([]), []);
  assert.deepEqual(momentPlies(undefined), []);
});

test("stepMoment finds the next and the previous moment, null at the ends", () => {
  const p = [5, 6, 9];
  assert.equal(stepMoment(p, 0, 1), 5);
  assert.equal(stepMoment(p, 5, 1), 6);
  assert.equal(stepMoment(p, 6, 1), 9);
  assert.equal(stepMoment(p, 9, 1), null);
  assert.equal(stepMoment(p, 9, -1), 6);
  assert.equal(stepMoment(p, 7, -1), 6);
  assert.equal(stepMoment(p, 5, -1), null);
  assert.equal(stepMoment([], 3, 1), null);
});

test("chartMarks keeps the moves that turn a game", () => {
  assert.deepEqual(
    chartMarks(moves).map((m) => m.ply),
    [5, 6, 7, 8]
  );
  assert.deepEqual(chartMarks(moves)[3], { ply: 8, cls: "brilliant" });
});

test("summaryRows lists the classes somebody has, best first", () => {
  const order = ["brilliant", "best", "book", "inaccuracy", "mistake", "blunder"];
  const rows = summaryRows([{ best: 12, mistake: 1 }, { best: 9, blunder: 2, book: 3 }], order);
  assert.deepEqual(rows, [
    { cls: "best", white: 12, black: 9 },
    { cls: "book", white: 0, black: 3 },
    { cls: "mistake", white: 1, black: 0 },
    { cls: "blunder", white: 0, black: 2 },
  ]);
  assert.deepEqual(summaryRows(undefined, order), []);
});

test("errorCount adds inaccuracies, mistakes, blunders and misses", () => {
  assert.equal(errorCount({ inaccuracy: 2, mistake: 1, blunder: 1, miss: 1, best: 10 }), 5);
  assert.equal(errorCount(undefined), 0);
});

test("moveLabel writes the move as in a score", () => {
  assert.equal(moveLabel({ ply: 13, color: "white", san: "Nf3" }), "7.Nf3");
  assert.equal(moveLabel({ ply: 14, color: "black", san: "Nf6" }), "7...Nf6");
  assert.equal(moveLabel(null), "");
});

const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

test("a FEN goes into an address and comes back", () => {
  const enc = encodeFen(START);
  assert.ok(!enc.includes(" "));
  assert.equal(decodeFen(enc), START);
  assert.equal(fenFromHash("#analysis/" + enc), START);
  assert.equal(fenFromHash("#analysis/" + encodeURIComponent(enc)), START);
  assert.equal(fenFromHash("#analysis"), "");
  assert.equal(fenFromHash("#analysis/hello"), "");
  assert.equal(fenFromHash("#review/abc"), "");
});

test("looksLikeFen tells a position from a game", () => {
  assert.ok(looksLikeFen(START));
  assert.ok(looksLikeFen("8/8/8/8/8/8/8/k6K b - -"));
  assert.ok(!looksLikeFen("1. e4 e5 2. Nf3 Nc6"));
  assert.ok(!looksLikeFen('[White "A"]\n[Black "B"]\n\n1. e4 e5'));
  assert.ok(!looksLikeFen("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP w KQkq - 0 1")); // seven ranks
  assert.ok(!looksLikeFen(""));
});

test("reviewUrl is an address that opens the review anywhere", () => {
  assert.equal(reviewUrl("https://chess.example", "/", "ab12"), "https://chess.example/#review/ab12");
});

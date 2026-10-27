import { test } from "node:test";
import assert from "node:assert/strict";
import { coachSay, dayLabel, hintSquare, judgeMove, keyAction, legalFor, pointsText, scoreView, signed, squaresOf } from "./puzzle.js";

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

test("the panel's numbers: the rating is main, the points are modest beside it", () => {
  const you = { rating: 1218, attempts: 31, solved: 21 };
  const pts = { total: 56, streak: 3, best: 5, today: 12 };
  const result = { counted: true, before: 1200, after: 1218, points: { gained: 5 } };
  const v = scoreView({ pointsMain: false, ratingVisible: true, you, pts, result });
  assert.deepEqual(v.main, { kind: "rating", value: 1218, change: 18, from: 1200 });
  assert.deepEqual(v.side, { kind: "points", value: 56, change: 5, streak: 3, today: 12 });
});

test("the points setting only puts the points first; the rating stays beside them", () => {
  const v = scoreView({
    pointsMain: true, ratingVisible: true, you: { rating: 1218 }, pts: { total: 56, streak: 3, today: 12 },
    result: { counted: true, before: 1200, after: 1218, points: { gained: 5 } },
  });
  assert.equal(v.main.kind, "points");
  assert.equal(v.main.change, 5);
  assert.equal(v.side.kind, "rating");
  assert.equal(v.side.value, 1218);
});

test("a failure moves the rating down and earns no points; a repeat changes nothing", () => {
  const you = { rating: 1174 };
  const pts = { total: 56, streak: 0, today: 12 };
  const lost = scoreView({ pointsMain: false, ratingVisible: true, you, pts, result: { counted: true, before: 1200, after: 1174, points: { gained: 0 } } });
  assert.equal(lost.main.change, -26);
  assert.equal(lost.side.change, 0);
  const again = scoreView({ pointsMain: false, ratingVisible: true, you, pts, result: { counted: false, before: 1174, after: 1174 } });
  assert.equal(again.main.change, null);
  assert.equal(again.side.change, null);
  const open = scoreView({ pointsMain: false, ratingVisible: true, you, pts, result: null });
  assert.equal(open.main.change, null);
});

test("with the rating hidden the points are all there is; with nothing, nothing", () => {
  const v = scoreView({ pointsMain: false, ratingVisible: false, you: { rating: 1200 }, pts: { total: 3, streak: 1, today: 3 }, result: null });
  assert.equal(v.main.kind, "points");
  assert.equal(v.side, null);
  assert.deepEqual(scoreView({ pointsMain: false, ratingVisible: true, you: null, pts: null, result: null }), { main: null, side: null });
  const guestNoPts = scoreView({ pointsMain: false, ratingVisible: true, you: { rating: 1200 }, pts: null, result: null });
  assert.equal(guestNoPts.main.kind, "rating");
  assert.equal(guestNoPts.side, null);
});

test("Dionis follows the state of the puzzle", () => {
  const base = { loading: false, phase: "solving", failed: false, idx: 1, mySide: "white" };
  assert.equal(coachSay(base).kind, "Ваш ход");
  assert.match(coachSay(base).text, /белых/);
  assert.match(coachSay({ ...base, mySide: "black" }).text, /чёрных/);
  assert.deepEqual([coachSay({ ...base, idx: 3 }).kind, coachSay({ ...base, idx: 3 }).tone], ["Верно!", "ok"]);
  assert.deepEqual([coachSay({ ...base, failed: true }).kind, coachSay({ ...base, failed: true }).tone], ["Не то", "bad"]);
  assert.equal(coachSay({ ...base, phase: "solved" }).tone, "ok");
  assert.equal(coachSay({ ...base, phase: "solved", failed: true }).tone, "warn");
  assert.equal(coachSay({ ...base, phase: "shown", failed: true }).kind, "Вот решение");
  assert.equal(coachSay({ ...base, phase: "intro" }).tone, "");
  assert.equal(coachSay({ ...base, loading: true }).kind, "Загружаю задачу…");
});

test("keys: Enter, Space and N go on; H hints; S shows the solution; the Russian layout works", () => {
  for (const k of ["Enter", " ", "n", "N", "т"]) assert.equal(keyAction(k), "next", k);
  for (const k of ["h", "H", "р"]) assert.equal(keyAction(k), "hint", k);
  for (const k of ["s", "S", "ы"]) assert.equal(keyAction(k), "solution", k);
  for (const k of ["a", "Escape", "Tab"]) assert.equal(keyAction(k), null, k);
  assert.deepEqual(["ArrowLeft", "ArrowRight", "Home", "End"].map(keyAction), ["back", "forward", "first", "last"]);
});

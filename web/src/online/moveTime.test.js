import test from "node:test";
import assert from "node:assert/strict";
import { averageTimes, formatMoveTime, hasMoveTimes, longestMoves } from "./moveTime.js";

test("formatMoveTime", () => {
  const cases = [
    [0, "0 с"],
    [20, "0 с"],
    [100, "0,1 с"],
    [2300, "2,3 с"],
    [5000, "5,0 с"],
    [9940, "9,9 с"],
    [9960, "10 с"],
    [23000, "23 с"],
    [59400, "59 с"],
    [65000, "1:05"],
    [600000, "10:00"],
    [3599000, "59:59"],
    [3600000, "1 ч"],
    [4800000, "1 ч 20 м"],
    [86400000, "1 д"],
    [(50 * 3600 + 20 * 60) * 1000, "2 д 2 ч"],
    [null, ""],
    [undefined, ""],
    [-1, ""],
  ];
  for (const [ms, want] of cases) assert.equal(formatMoveTime(ms), want, String(ms));
});

test("formatMoveTime follows the units of the language", () => {
  const en = { s: "s", m: "m", h: "h", d: "d", dec: "." };
  assert.equal(formatMoveTime(2300, en), "2.3 s");
  assert.equal(formatMoveTime(4800000, en), "1 h 20 m");
  assert.equal(formatMoveTime(90000000, en), "1 d 1 h");
});

const moves = [
  { ply: 1, color: "white", timeMs: 1000 },
  { ply: 2, color: "black", timeMs: 8000 },
  { ply: 3, color: "white", timeMs: 30000 },
  { ply: 4, color: "black", timeMs: 2000 },
  { ply: 5, color: "white" },
];

test("averageTimes counts only the moves with a time", () => {
  assert.deepEqual(averageTimes(moves), { white: 15500, black: 5000 });
  assert.deepEqual(averageTimes([{ ply: 1, color: "white" }]), { white: null, black: null });
  assert.deepEqual(averageTimes(null), { white: null, black: null });
});

test("longestMoves lists the long ones, longest first", () => {
  assert.deepEqual(longestMoves(moves), [3, 2]);
  assert.deepEqual(longestMoves(moves, 1), [3]);
  assert.deepEqual(longestMoves(moves, 3, 500), [3, 2, 4]);
  assert.deepEqual(longestMoves([{ ply: 1, color: "white" }]), []);
});

test("hasMoveTimes", () => {
  assert.equal(hasMoveTimes(moves), true);
  assert.equal(hasMoveTimes([{ ply: 1 }]), false);
  assert.equal(hasMoveTimes(undefined), false);
});

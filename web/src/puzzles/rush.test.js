import test from "node:test";
import assert from "node:assert/strict";
import { RUSH_MODES, clockText, runSummary, targetRating } from "./rush.js";

test("the puzzles get harder with every one solved, up to a ceiling", () => {
  assert.equal(targetRating(0), 500);
  assert.equal(targetRating(10), 1000);
  assert.ok(targetRating(11) > targetRating(10));
  assert.equal(targetRating(500), 2800);
  assert.equal(targetRating(-3), 500);
});

test("the clock shows minutes and seconds and never goes below zero", () => {
  assert.equal(clockText(RUSH_MODES["3min"].ms), "3:00");
  assert.equal(clockText(179_001), "3:00");
  assert.equal(clockText(59_000), "0:59");
  assert.equal(clockText(61_000), "1:01");
  assert.equal(clockText(1), "0:01");
  assert.equal(clockText(0), "0:00");
  assert.equal(clockText(-500), "0:00");
});

test("a run is told by its reason, and only a better score than before is a record", () => {
  assert.deepEqual(runSummary("mistake", 12, 9), { headline: "Ошибка: решено 12", record: true });
  assert.deepEqual(runSummary("time", 9, 9), { headline: "Время вышло: решено 9", record: false });
  assert.equal(runSummary("mistake", 0, 0).record, false);
  assert.equal(runSummary("quit", 5, 0).headline, "Серия окончена: решено 5");
});

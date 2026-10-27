import test from "node:test";
import assert from "node:assert/strict";
import {
  CATEGORY_ORDER,
  MAX_SINGLE_TOASTS,
  groupByCategory,
  latest,
  mayAsk,
  overallPercent,
  planToasts,
  progressPercent,
  showsBar,
  tierTotals,
} from "./logic.js";

const item = (id, category, tier, extra = {}) => ({ id, category, tier, target: 1, progress: 0, unlocked: false, ...extra });

test("badges are grouped in the order of the page, unknown categories go last", () => {
  const items = [
    item("a", "puzzles", "gold", { unlocked: true }),
    item("b", "games", "bronze"),
    item("c", "games", "silver", { unlocked: true }),
    item("d", "newthing", "bronze"),
  ];
  const g = groupByCategory(items);
  assert.deepEqual(g.map((x) => x.category), ["games", "puzzles", "newthing"]);
  assert.deepEqual(g[0].items.map((i) => i.id), ["b", "c"]);
  assert.equal(g[0].unlocked, 1);
  assert.equal(g[0].total, 2);
  assert.deepEqual(groupByCategory([]), []);
  assert.deepEqual(groupByCategory(null), []);
  assert.equal(CATEGORY_ORDER.length, new Set(CATEGORY_ORDER).size);
});

test("totals by tier", () => {
  const t = tierTotals([item("a", "x", "bronze", { unlocked: true }), item("b", "x", "bronze"), item("c", "x", "gold"), item("d", "x", "weird")]);
  assert.deepEqual(t.bronze, { n: 1, total: 2 });
  assert.deepEqual(t.silver, { n: 0, total: 0 });
  assert.deepEqual(t.gold, { n: 0, total: 1 });
});

test("the bar: earned is full, yes-or-no has none, the rest is a held percent", () => {
  assert.equal(progressPercent(item("a", "x", "gold", { unlocked: true, target: 100, progress: 100 })), 100);
  assert.equal(progressPercent(item("a", "x", "gold", { target: 1 })), 0);
  assert.equal(progressPercent(item("a", "x", "gold", { target: 100, progress: 37 })), 37);
  assert.equal(progressPercent(item("a", "x", "gold", { target: 3, progress: 1 })), 33);
  assert.equal(progressPercent(item("a", "x", "gold", { target: 10, progress: 99 })), 100);
  assert.equal(progressPercent(item("a", "x", "gold", { target: 10, progress: -4 })), 0);
  assert.equal(progressPercent(null), 0);
  assert.equal(showsBar({ target: 1 }), false);
  assert.equal(showsBar({ target: 2 }), true);
  assert.equal(showsBar(undefined), false);
});

test("overall percent", () => {
  assert.equal(overallPercent(0, 0), 0);
  assert.equal(overallPercent(12, 48), 25);
  assert.equal(overallPercent(48, 48), 100);
  assert.equal(overallPercent(5, 3), 100);
});

test("the latest badges of a public list, newest first, at most six", () => {
  const items = [1, 2, 3, 4, 5, 6, 7, 8].map((n) => ({ id: "b" + n, unlockedAt: `2026-10-0${n}T10:00:00Z` }));
  const l = latest(items);
  assert.equal(l.length, 6);
  assert.equal(l[0].id, "b8");
  assert.equal(l[5].id, "b3");
  assert.deepEqual(latest(null), []);
  assert.equal(latest(items, 2).length, 2);
});

test("notices: one by one up to three, then one for all; shown ones are skipped", () => {
  assert.deepEqual(planToasts([], new Set()), []);
  const one = planToasts(["a"]);
  assert.deepEqual(one, [{ key: "a", id: "a", ids: ["a"] }]);
  const three = planToasts(["a", "b", "c"]);
  assert.equal(three.length, MAX_SINGLE_TOASTS);
  const many = planToasts(["a", "b", "c", "d", "e"]);
  assert.equal(many.length, 1);
  assert.equal(many[0].many, 5);
  assert.deepEqual(many[0].ids, ["a", "b", "c", "d", "e"]);
  // the ones already shown do not come back, and the count follows what is left
  assert.deepEqual(planToasts(["a", "b"], new Set(["a"])).map((x) => x.id), ["b"]);
  assert.deepEqual(planToasts(["a", "b"], new Set(["a", "b"])), []);
});

test("the notice does not ask the server more often than the gap", () => {
  assert.equal(mayAsk(100_000, 0), true);
  assert.equal(mayAsk(100_000, 90_000), false);
  assert.equal(mayAsk(100_000, 85_000), true);
  assert.equal(mayAsk(100_000, 99_999, 0), true);
});

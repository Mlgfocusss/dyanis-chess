import test from "node:test";
import assert from "node:assert/strict";
import { categoryOf, changeLine, deltaText, ratingText, searchLine } from "./rating.js";

test("games are sorted into bullet, blitz and rapid by the clock, as on the server", () => {
  assert.equal(categoryOf(1, 0), "bullet");
  assert.equal(categoryOf(2, 1), "bullet"); // 2 min + 40 s
  assert.equal(categoryOf(3, 0), "blitz");
  assert.equal(categoryOf(3, 2), "blitz");
  assert.equal(categoryOf(5, 0), "blitz");
  assert.equal(categoryOf(10, 0), "rapid");
  assert.equal(categoryOf(10, 5), "rapid");
  assert.equal(categoryOf(30, 0), "rapid");
});

test("a rough rating carries a question mark", () => {
  assert.equal(ratingText(1500, false), "1500");
  assert.equal(ratingText(1200, true), "1200?");
});

test("a change of rating is signed", () => {
  assert.equal(deltaText(1200, 1215), "+15");
  assert.equal(deltaText(1200, 1192), "−8");
  assert.equal(deltaText(1200, 1200), "±0");
  assert.equal(changeLine({ category: "blitz", before: 1200, after: 1215, provisional: true }), "Блиц: 1200 → 1215? (+15)");
  assert.equal(changeLine({ category: "duel", before: 1300, after: 1290 }), "Дуэль: 1300 → 1290 (−10)");
  assert.equal(changeLine(null), "");
});

test("the wait is told with the window and the queue", () => {
  assert.equal(searchLine({ waited: 12, window: 400, count: 3 }), "12 с · разница в рейтинге до 400 · в очереди: 3");
  assert.equal(searchLine({ waited: 0 }), "0 с");
  assert.equal(searchLine(null), "");
});

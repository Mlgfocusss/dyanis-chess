import { test } from "node:test";
import assert from "node:assert/strict";
import { nextScrollTop } from "./scroll.js";

const base = { viewTop: 100, viewHeight: 200, itemHeight: 30, scrollTop: 50 };

test("an item that is fully visible does not move the box", () => {
  assert.equal(nextScrollTop({ ...base, itemTop: 150 }), 50);
  assert.equal(nextScrollTop({ ...base, itemTop: 100 }), 50);
  assert.equal(nextScrollTop({ ...base, itemTop: 270 }), 50); // ends exactly at the bottom edge
});

test("an item below the view is brought in at the bottom edge", () => {
  assert.equal(nextScrollTop({ ...base, itemTop: 300 }), 50 + 30); // ends 30 px under the edge
});

test("an item above the view is brought in at the top edge", () => {
  assert.equal(nextScrollTop({ ...base, itemTop: 80 }), 50 - 20);
});

test("an item taller than the view shows its start", () => {
  assert.equal(nextScrollTop({ ...base, itemTop: 150, itemHeight: 400 }), 50 + 50, "the start of the item is not pushed out of sight");
});

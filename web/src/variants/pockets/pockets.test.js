import test from "node:test";
import assert from "node:assert/strict";
import {
  POCKET_ORDER,
  dropAction,
  dropClick,
  dropDots,
  dropSquares,
  droppableKinds,
  kindOfCode,
  parseDrop,
  pocketCounts,
  pocketTotal,
  promotedClasses,
  sideOfCode,
} from "./pockets.js";

const legal = ["e2e4", "N@f3", "N@g5", "P@e4", "Q@a8", "g1f3"];

test("a drop is read and written the same way", () => {
  assert.deepEqual(parseDrop("N@f3"), { kind: "N", square: "f3" });
  assert.equal(parseDrop("e2e4"), null);
  assert.equal(parseDrop("K@e4"), null);
  assert.equal(parseDrop("N@i3"), null);
  assert.equal(parseDrop(undefined), null);
  assert.equal(dropAction("Q", "d5"), "Q@d5");
});

test("the squares and the kinds of the legal drops", () => {
  assert.deepEqual(dropSquares(legal, "N"), ["f3", "g5"]);
  assert.deepEqual(dropSquares(legal, "R"), []);
  assert.deepEqual([...droppableKinds(legal)].sort(), ["N", "P", "Q"]);
  assert.deepEqual(dropDots(legal, "N"), { f3: "dot", g5: "dot" });
  assert.equal(dropDots(legal, "R"), undefined);
  assert.equal(dropDots(legal, null), undefined);
});

test("a click while a piece is chosen is a drop only on a legal square", () => {
  assert.equal(dropClick(legal, "N", "f3"), "N@f3");
  assert.equal(dropClick(legal, "N", "a1"), null);
  assert.equal(dropClick(legal, null, "f3"), null);
  assert.equal(dropClick([], "N", "f3"), null);
});

test("the pocket counts are filled in for every kind", () => {
  const c = pocketCounts({ white: { N: 2, P: 1 }, black: {} }, "white");
  assert.deepEqual(Object.keys(c), POCKET_ORDER);
  assert.equal(c.N, 2);
  assert.equal(c.Q, 0);
  assert.equal(pocketTotal(c), 3);
  assert.equal(pocketTotal(pocketCounts(undefined, "black")), 0);
  assert.equal(pocketCounts({ white: { N: -3 } }, "white").N, 0);
});

test("piece codes of the board", () => {
  assert.equal(kindOfCode("wN"), "N");
  assert.equal(kindOfCode("bq"), "Q");
  assert.equal(kindOfCode(""), "");
  assert.equal(sideOfCode("wN"), "white");
  assert.equal(sideOfCode("bP"), "black");
  assert.equal(sideOfCode("xD"), "");
});

test("promoted pieces get a class", () => {
  assert.deepEqual(promotedClasses({ promoted: ["a8", "c1"] }), { a8: "sq-promoted", c1: "sq-promoted" });
  assert.deepEqual(promotedClasses(undefined), {});
});

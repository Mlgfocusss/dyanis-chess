import { test } from "node:test";
import assert from "node:assert/strict";
import { START_FEN, buildTree, childWith, fenAt, learnedPercent, lineText, pathTo, statusOf, whenText, whoMoves } from "./tree.js";

const mv = (id, parent, uci, san, ply, mine, extra = {}) => ({ id, parent, uci, san, ply, mine, fen: "fen" + id, box: 0, due: null, ...extra });
const moves = [
  mv(1, 0, "e2e4", "e4", 1, true),
  mv(2, 1, "e7e5", "e5", 2, false),
  mv(3, 2, "g1f3", "Nf3", 3, true),
  mv(4, 1, "c7c5", "c5", 2, false),
];
const tree = buildTree(moves);

test("paths, positions and children", () => {
  assert.deepEqual(pathTo(tree, 3).map((m) => m.san), ["e4", "e5", "Nf3"]);
  assert.deepEqual(pathTo(tree, 0), []);
  assert.deepEqual(pathTo(tree, 99), []);
  assert.equal(fenAt(tree, 0), START_FEN);
  assert.equal(fenAt(tree, 2), "fen2");
  assert.equal(childWith(tree, 1, "c7c5").id, 4);
  assert.equal(childWith(tree, 1, "d7d5"), null);
  assert.equal(tree.children.get(1).length, 2);
});

test("lines are written as in a score", () => {
  assert.equal(lineText(pathTo(tree, 3)), "1. e4 e5 2. Nf3");
  assert.equal(lineText([moves[1]]), "1... e5");
  assert.equal(lineText([]), "");
});

test("whose move it is", () => {
  assert.equal(whoMoves("white", 0, tree), "mine"); // the start position, White to move
  assert.equal(whoMoves("white", 1, tree), "opponent");
  assert.equal(whoMoves("black", 0, tree), "opponent");
  assert.equal(whoMoves("black", 1, tree), "mine");
});

test("the status of a move", () => {
  const now = Date.parse("2026-10-15T12:00:00Z");
  assert.equal(statusOf(moves[1], now), "opponent");
  assert.equal(statusOf(moves[0], now), "new");
  assert.equal(statusOf({ ...moves[0], due: "2026-10-15T11:00:00Z", box: 1 }, now), "due");
  assert.equal(statusOf({ ...moves[0], due: "2026-10-16T11:00:00Z", box: 1 }, now), "learning");
  assert.equal(statusOf({ ...moves[0], due: "2026-10-16T11:00:00Z", box: 5 }, now), "learned");
});

test("progress and waiting times in words", () => {
  assert.equal(learnedPercent({ mine: 8, learned: 2 }), 25);
  assert.equal(learnedPercent({ mine: 0, learned: 0 }), 0);
  const now = Date.parse("2026-10-15T12:00:00Z");
  assert.equal(whenText("2026-10-15T12:10:00Z", now), "через 10 мин");
  assert.equal(whenText("2026-10-15T15:00:00Z", now), "через 3 ч");
  assert.equal(whenText("2026-10-16T12:00:00Z", now), "завтра");
  assert.equal(whenText("2026-10-20T12:00:00Z", now), "через 5 дн.");
  assert.equal(whenText("2026-10-15T11:00:00Z", now), "сейчас");
});

test("counts, gaps and subtrees follow the moves in hand", async () => {
  const { countsOf, gapsOf, subtreeIds } = await import("./tree.js");
  const now = Date.parse("2026-10-15T12:00:00Z");
  const ms = [
    mv(1, 0, "e2e4", "e4", 1, true, { due: "2026-10-15T11:00:00Z", box: 1 }),
    mv(2, 1, "e7e5", "e5", 2, false),
    mv(3, 2, "g1f3", "Nf3", 3, true, { due: "2026-11-15T11:00:00Z", box: 5 }),
    mv(4, 1, "c7c5", "c5", 2, false),
    mv(5, 4, "g1f3", "Nf3", 3, true),
  ];
  assert.deepEqual(countsOf(ms, now), { moves: 5, mine: 3, new: 1, due: 1, learned: 1 });
  const t = buildTree(ms);
  assert.deepEqual(gapsOf(t), []); // every line ends with my move
  const t2 = buildTree([...ms, mv(6, 5, "d7d6", "d6", 4, false)]);
  assert.deepEqual(gapsOf(t2).map((m) => m.id), [6]);
  assert.deepEqual(subtreeIds(t, 2).sort(), [2, 3]);
  assert.deepEqual(subtreeIds(t, 1).sort(), [1, 2, 3, 4, 5]);
});

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

// ---- transpositions: the same examples as backend/internal/repertoire/positions_test.go
test("a position is told by the pieces, the turn, castling and a usable en passant square", async () => {
  const { posKey } = await import("./tree.js");
  const start = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";
  assert.equal(posKey(start), "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -");
  assert.equal(posKey(start), posKey("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 12 40"));
  // 1.e4 c5 2.Nf3 and 1.Nf3 c5 2.e4: the FENs differ by an en passant square nobody can use
  const a = "rnbqkbnr/pp1ppppp/8/2p5/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2";
  const b = "rnbqkbnr/pp1ppppp/8/2p5/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq e3 0 2";
  assert.notEqual(a, b);
  assert.equal(posKey(a), posKey(b));
  // a square a pawn can use stays; one it cannot is dropped
  assert.ok(posKey("rnbqkbnr/1pp1pppp/p7/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3").endsWith(" d6"));
  assert.ok(posKey("rnbqkbnr/1pp1pppp/p7/3p4/4P3/P7/1PPP1PPP/RNBQKBNR w KQkq d6 0 3").endsWith(" -"));
  assert.ok(posKey("rnbqkbnr/ppppppp1/8/8/P5Pp/8/1PPPPP1P/RNBQKBNR b KQkq g3 0 3").endsWith(" g3"));
  assert.equal(posKey(""), "");
  assert.equal(posKey("junk"), "junk");
});

// 1.d4 Nf6 2.c4 e6 3.Nc3 (ids 1-5) and 1.c4 e6 2.d4 Nf6 (ids 6-9); 4 and 9 are one position
const POS = "rnbqkb1r/pppp1ppp/4pn2/8/2PP4/8/PP2PPPP/RNBQKBNR w KQkq - 0 3";
const twoOrders = [
  mv(1, 0, "d2d4", "d4", 1, true),
  mv(2, 1, "g8f6", "Nf6", 2, false),
  mv(3, 2, "c2c4", "c4", 3, true),
  mv(4, 3, "e7e6", "e6", 4, false, { fen: POS }),
  mv(5, 4, "b1c3", "Nc3", 5, true),
  mv(6, 0, "c2c4", "c4", 1, true),
  mv(7, 6, "e7e6", "e6", 2, false),
  mv(8, 7, "d2d4", "d4", 3, true),
  mv(9, 8, "g8f6", "Nf6", 4, false, { fen: POS.replace(" 0 3", " 5 9") }),
];

test("the replies of a position include those of the other order of moves", async () => {
  const { repliesAt, replyWith, samePosition, twinsOf, repOf, transposedFrom } = await import("./tree.js");
  const t = buildTree(twoOrders);
  assert.ok(samePosition(t, 4, 9));
  assert.ok(!samePosition(t, 4, 8));
  assert.deepEqual(twinsOf(t, 9), [4]);
  assert.equal(repOf(t, 9), 4);
  assert.deepEqual(repliesAt(t, 9).map((m) => m.id), [5]);
  assert.equal(replyWith(t, 9, "b1c3").id, 5);
  assert.equal(replyWith(t, 9, "b1c3").parent, 4); // found on the other branch
  assert.equal(transposedFrom(t, 9), 4);
  assert.equal(transposedFrom(t, 4), null);
  // its own moves come first
  const more = buildTree([...twoOrders, mv(10, 9, "g1f3", "Nf3", 5, true)]);
  assert.deepEqual(repliesAt(more, 4).map((m) => m.uci), ["b1c3", "g1f3"]);
  assert.deepEqual(repliesAt(more, 9).map((m) => m.uci), ["g1f3", "b1c3"]);
  assert.equal(transposedFrom(more, 9), null);
});

test("counts, cards and holes count a position once", async () => {
  const { countsOf, gapsOf, cardsOf } = await import("./tree.js");
  // the second order is not a hole: the repository answers the position (3.Nc3)
  assert.deepEqual(gapsOf(buildTree(twoOrders)), []);
  // without the answer both orders end in one position: one hole
  assert.deepEqual(gapsOf(buildTree(twoOrders.slice(0, 4).concat(twoOrders.slice(5)))).map((m) => m.id), [4]);
  // the same 3.Nc3 reached by both orders is one card, the more asked node stands for it
  const dup = [...twoOrders, mv(10, 9, "b1c3", "Nc3", 5, true, { reviews: 2, due: "2026-10-15T11:00:00Z", box: 1 })];
  const cards = cardsOf(buildTree(dup));
  assert.equal(cards.length, 5);
  const nc3 = cards.find((c) => c.rep.uci === "b1c3");
  assert.deepEqual(nc3.members, [5, 10]);
  assert.equal(nc3.rep.id, 10);
  const now = Date.parse("2026-10-15T12:00:00Z");
  assert.deepEqual(countsOf(dup, now), { moves: 9, mine: 5, new: 4, due: 1, learned: 0 });
});

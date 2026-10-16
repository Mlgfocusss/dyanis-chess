import test from "node:test";
import assert from "node:assert/strict";
import { castlingOf, castlingSquares, piecesOf, premoveList, premoveMoves, premoveToPlay, previewPlacement, queuedPosition, resolveMove, targetSquares } from "./boardrules.js";

const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";
const CASTLE = "r3k2r/pppqbppp/2n1bn2/3pp3/3PP3/2N1BN2/PPPQBPPP/R3K2R w KQkq - 0 1";
const sorted = (a) => [...a].sort();

test("a castling is told from the king's move", () => {
  assert.deepEqual(castlingOf(CASTLE, "e1g1"), { rook: "h1", to: "f1" });
  assert.deepEqual(castlingOf(CASTLE, "e1c1"), { rook: "a1", to: "d1" });
  assert.equal(castlingOf(CASTLE, "e1f1"), null);
  assert.equal(castlingOf(CASTLE, "a1a2"), null);
  assert.equal(castlingOf("r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1", "e8c8")?.rook, "a8");
  // a king that is not on its home square makes no castling
  assert.equal(castlingOf("8/8/8/8/8/8/8/5K2 w - - 0 1", "f1h1"), null);
});

test("the way of castling decides which squares mean it", () => {
  assert.deepEqual(castlingSquares(CASTLE, "e1g1", "standard"), ["g1"]);
  assert.deepEqual(castlingSquares(CASTLE, "e1g1", "rook"), ["h1"]);
  assert.deepEqual(castlingSquares(CASTLE, "e1g1", "both"), ["g1", "h1"]);
  assert.equal(castlingSquares(CASTLE, "e1d1", "both"), null);
});

test("a move is resolved by the squares the person used", () => {
  const legal = [{ uci: "e1g1" }, { uci: "e1c1" }, { uci: "e1f1" }, { uci: "e1d1" }, { uci: "a2a3" }];
  assert.equal(resolveMove(legal, CASTLE, "e1", "g1", "standard"), "e1g1");
  assert.equal(resolveMove(legal, CASTLE, "e1", "h1", "standard"), null); // the rook way is off
  assert.equal(resolveMove(legal, CASTLE, "e1", "h1", "rook"), "e1g1");
  assert.equal(resolveMove(legal, CASTLE, "e1", "g1", "rook"), null); // two squares is not castling in this mode
  assert.equal(resolveMove(legal, CASTLE, "e1", "a1", "both"), "e1c1");
  assert.equal(resolveMove(legal, CASTLE, "e1", "f1", "rook"), "e1f1"); // an ordinary king move
  assert.equal(resolveMove(legal, START, "a2", "a3", "rook"), "a2a3");
  assert.equal(resolveMove(legal, START, "a2", "a5"), null);
  assert.deepEqual(sorted(targetSquares(legal, CASTLE, "e1", "both")), ["a1", "c1", "d1", "f1", "g1", "h1"]);
  assert.deepEqual(sorted(targetSquares(legal, CASTLE, "e1", "rook")), ["a1", "d1", "f1", "h1"]);
});

test("a preview shows the move, with the rook of a castling, en passant and a promotion", () => {
  assert.equal(previewPlacement(START, "e2e4"), "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR");
  assert.equal(previewPlacement(CASTLE, "e1g1"), "r3k2r/pppqbppp/2n1bn2/3pp3/3PP3/2N1BN2/PPPQBPPP/R4RK1");
  assert.equal(previewPlacement(CASTLE, "e1c1"), "r3k2r/pppqbppp/2n1bn2/3pp3/3PP3/2N1BN2/PPPQBPPP/2KR3R");
  // white pawn e5 takes d6 en passant: the black pawn on d5 goes
  assert.equal(previewPlacement("4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 1", "e5d6"), "4k3/8/3P4/8/8/8/8/4K3");
  // a promotion with a capture, both colours
  assert.equal(previewPlacement("1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7b8n"), "1N2k3/8/8/8/8/8/8/4K3");
  assert.equal(previewPlacement("4k3/8/8/8/8/8/p7/4K3 b - - 0 1", "a2a1q"), "4k3/8/8/8/8/8/8/q3K3");
  assert.equal(previewPlacement(START, "e4e5"), piecesOf(START) && "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR"); // nothing on e4: unchanged
});

test("premoves follow the geometry and stop at the person's own pieces", () => {
  // a pawn also premoves onto its diagonals: a piece may arrive there
  assert.deepEqual(sorted(premoveMoves(START, "e2")), ["e2d3", "e2e3", "e2e4", "e2f3"]);
  assert.deepEqual(sorted(premoveMoves(START, "g1")), ["g1f3", "g1h3"]);
  assert.deepEqual(premoveMoves(START, "a1"), []); // shut in by its own pieces
  assert.deepEqual(premoveMoves(START, "e4"), []); // no piece
  // a rook goes through the opponent's pieces (they may move) and stops before its own
  const fen = "4k3/8/8/8/4p3/8/4R3/4K3 w - - 0 1";
  assert.deepEqual(sorted(premoveMoves(fen, "e2")), ["e2a2", "e2b2", "e2c2", "e2d2", "e2e3", "e2e4", "e2e5", "e2e6", "e2e7", "e2e8", "e2f2", "e2g2", "e2h2"]);
  // a pawn takes diagonally even where nothing stands (the piece may arrive), and promotes to a queen
  assert.deepEqual(sorted(premoveMoves("4k3/P7/8/8/8/8/8/4K3 w - - 0 1", "a7")), ["a7a8q", "a7b8q"]);
  // castling is premoved only with the rook at home and the way clear of own pieces
  assert.ok(premoveMoves(CASTLE, "e1").includes("e1g1") && premoveMoves(CASTLE, "e1").includes("e1c1"));
  assert.ok(!premoveMoves("r3k2r/8/8/8/8/8/8/R2QK2R w KQkq - 0 1", "e1").includes("e1c1"));
  // black pawns go down
  assert.deepEqual(sorted(premoveMoves(START, "d7")), ["d7c6", "d7d5", "d7d6", "d7e6"]);
});

test("the premoves of a side are the moves of its pieces only", () => {
  const w = premoveList(START, "w").map((m) => m.uci);
  assert.equal(w.length, 34); // 8 pawns (2 forward, 1 or 2 diagonals) and 2 knights (2 each)
  assert.ok(w.every((u) => "12".includes(u[1])));
  assert.ok(premoveList(START, "b").every((m) => "78".includes(m.uci[1])));
});

test("a premove is played when it is legal, a promotion as a queen", () => {
  assert.equal(premoveToPlay([{ uci: "e2e4" }, { uci: "d2d4" }], "e2e4"), "e2e4");
  assert.equal(premoveToPlay([{ uci: "d2d4" }], "e2e4"), null);
  assert.equal(premoveToPlay([{ uci: "a7a8q" }, { uci: "a7a8n" }], "a7a8"), "a7a8q");
  assert.equal(premoveToPlay(["e2e4"], "e2e4"), "e2e4");
  assert.equal(premoveToPlay([{ uci: "e2e4" }], null), null);
});

test("several premoves are chosen one after another, each on the position the earlier ones leave", () => {
  assert.equal(queuedPosition(START, []), START);
  assert.equal(queuedPosition(START, undefined), START);
  const two = queuedPosition(START, ["e2e4", "g1f3"]);
  assert.equal(two.split(" ")[0], "rnbqkbnr/pppppppp/8/8/4P3/5N2/PPPP1PPP/RNBQKB1R");
  // the third is chosen on that position: the knight on f3 can go to e5, the pawn on e4 to e5
  assert.ok(premoveMoves(two, "f3").includes("f3e5"));
  assert.deepEqual(sorted(premoveMoves(two, "e4")), ["e4d5", "e4e5", "e4f5"]);
  // a castling after the knight and the bishop were moved out
  const ready = queuedPosition(START, ["e2e4", "g1f3", "f1c4"]);
  assert.ok(premoveMoves(ready, "e1").includes("e1g1"));
  // a pawn push is not a capture: a piece beside it stays put, even one of its own side
  assert.equal(previewPlacement("4k3/8/8/8/8/8/PN6/4K3 w - - 0 1", "a2b3").split("/")[6], "1N6");
  assert.equal(previewPlacement("4k3/8/8/8/8/8/PP6/4K3 w - - 0 1", "a2b3").split("/")[6], "1P6");
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { buildTree } from "./tree.js";
import { coveragePercent, formatWeight, gapCounts, isPopularReply, keyOf, markedNodes, moveLabel, nextGap, sourceOf, treeSignature, visibleGaps } from "./gaps.js";

// 1.e4 (mine) e5 2.Nf3 (mine); 1...c5 (the opponent's, no answer yet)
const mv = (id, parent, uci, san, ply, mine) => ({ id, parent, uci, san, fen: `fen${id}`, ply, mine });
const moves = [mv(1, 0, "e2e4", "e4", 1, true), mv(2, 1, "e7e5", "e5", 2, false), mv(3, 2, "g1f3", "Nf3", 3, true), mv(4, 1, "c7c5", "c5", 2, false)];
const tree = buildTree(moves);

const missing = (at, ply, uci, san, weight) => ({ kind: "missing", at, ply, fen: "f", path: [], uci, san, pct: 10, games: 100, weight, source: "lichess" });
const answer = (at, ply, uci, san, weight) => ({ kind: "answer", at, ply, fen: "f", path: [], uci, san, pct: 20, games: 200, weight, source: "lichess" });

test("moves are written with their number", () => {
  assert.equal(moveLabel(0, "e4"), "1. e4");
  assert.equal(moveLabel(1, "c5"), "1... c5");
  assert.equal(moveLabel(4, "Nf3"), "3. Nf3");
  assert.equal(moveLabel(5, "a6"), "3... a6");
});

test("an unanswered line that the search did not list is still a hole, of unknown weight, and goes last", () => {
  const gaps = visibleGaps(null, tree);
  assert.deepEqual(gaps.map(keyOf), ["answer:4:c7c5"]);
  assert.equal(gaps[0].weight, null);
  const scan = { gaps: [missing(1, 1, "c7c6", "c6", 0.08), missing(0, 0, "e2e4", "e4", 0.5), answer(4, 2, "c7c5", "c5", 0.3)] };
  // the first move missing at the start is not a hole to show when the start already has a move
  const shown = visibleGaps(scan, tree);
  assert.deepEqual(shown.map((g) => g.san), ["c5", "c6"]); // e4 is a child of the start: resolved
});

test("holes the tree has since filled are dropped, in both kinds", () => {
  const scan = { gaps: [missing(1, 1, "e7e5", "e5", 0.4), missing(1, 1, "c7c6", "c6", 0.08), answer(4, 2, "c7c5", "c5", 0.3), missing(99, 3, "a7a6", "a6", 0.1)] };
  const shown = visibleGaps(scan, tree);
  assert.deepEqual(shown.map((g) => g.san), ["c5", "c6"]); // e5 is in the tree; node 99 is gone
  const answered = buildTree([...moves, mv(5, 4, "g1f3", "Nf3", 3, true)]);
  assert.deepEqual(visibleGaps(scan, answered).map((g) => g.san), ["c6"]);
});

test("order: heavier first, then the earlier ply; the empty start is not a hole to list", () => {
  const scan = {
    gaps: [missing(2, 2, "b8c6", "Nc6", 0.2), missing(1, 1, "c7c6", "c6", 0.2), missing(1, 1, "d7d5", "d5", 0.3), answer(0, 0, "", "", 1)],
  };
  assert.deepEqual(visibleGaps(scan, tree).map((g) => g.san), ["d5", "c6", "Nc6", "c5"]);
  assert.deepEqual(visibleGaps({ gaps: [answer(0, 0, "", "", 1)] }, buildTree([])), []);
});

test("nodes to mark, the next hole going round, the figures", () => {
  const gaps = [missing(1, 1, "c7c6", "c6", 0.3), answer(4, 2, "c7c5", "c5", 0.2), missing(0, 0, "d2d4", "d4", 0.1)];
  assert.deepEqual([...markedNodes(gaps)].sort(), [1, 4]);
  assert.equal(nextGap([], "x"), null);
  assert.equal(keyOf(nextGap(gaps, null)), keyOf(gaps[0]));
  assert.equal(keyOf(nextGap(gaps, keyOf(gaps[0]))), keyOf(gaps[1]));
  assert.equal(keyOf(nextGap(gaps, keyOf(gaps[2]))), keyOf(gaps[0])); // round
  assert.equal(keyOf(nextGap(gaps, "gone")), keyOf(gaps[0]));
  assert.deepEqual(gapCounts(gaps), { missing: 2, answer: 1, total: 3 });
});

test("coverage never claims 100% while holes are open", () => {
  assert.equal(coveragePercent(0.123), 12);
  assert.equal(coveragePercent(0.999, 2), 99);
  assert.equal(coveragePercent(1, 0), 100);
  assert.equal(coveragePercent(undefined), 0);
});

test("a chance as a percentage", () => {
  assert.equal(formatWeight(0.374), "37%");
  assert.equal(formatWeight(0.0834), "8.3%".replace(".", ","));
  assert.equal(formatWeight(0.0004), "<0,1%");
  assert.equal(formatWeight(null), "");
  assert.equal(formatWeight(0), "0%");
});

test("the rule for a popular reply is the server's: 5% and 50 games, and not already in the repository", () => {
  const stat = (games, share, repo = null) => ({ repo, stat: true, games, share });
  assert.equal(isPopularReply(stat(500, 0.3), true), true);
  assert.equal(isPopularReply(stat(500, 0.049), true), false);
  assert.equal(isPopularReply(stat(49, 0.3), true), false);
  assert.equal(isPopularReply(stat(500, 0.3, { id: 1 }), true), false);
  assert.equal(isPopularReply({ repo: null, theory: true, share: 0.2 }, false), true);
  assert.equal(isPopularReply({ repo: null, theory: true, share: 0.02 }, false), false);
  assert.equal(isPopularReply({ repo: null, theory: false, stat: false, share: 0.9 }, false), false);
});

test("source of a search, and the signature of a tree", () => {
  assert.equal(sourceOf(null), "");
  assert.equal(sourceOf({ source: "lichess" }), "lichess");
  assert.equal(sourceOf({ source: "mixed" }), "mixed");
  assert.equal(sourceOf({ source: "anything" }), "theory");
  assert.equal(treeSignature(moves), "4:4");
  assert.notEqual(treeSignature(moves), treeSignature(moves.slice(0, 3)));
});

import { cardHoles, listTotals } from "./gaps.js";
import { PRESETS, presetById, presetsFor } from "./presets.js";

test("a card counts the open lines always, the missing replies only after a search", () => {
  assert.deepEqual(cardHoles({ open: 2, scanned: false, missing: 0, coverage: 0 }), { holes: 2, checked: false, stale: false, partial: false, coverage: null, source: "" });
  const c = cardHoles({ open: 2, scanned: true, missing: 3, coverage: 0.42, partial: true, stale: true, source: "lichess" });
  assert.deepEqual(c, { holes: 5, checked: true, stale: true, partial: true, coverage: 42, source: "lichess" });
  assert.equal(cardHoles({ open: 0, scanned: true, missing: 0, coverage: 1 }).coverage, 100);
  assert.equal(cardHoles({ open: 1, scanned: true, missing: 0, coverage: 1 }).coverage, 99);
  assert.equal(cardHoles(undefined).holes, 0);
});

test("the strip of the list adds the cards up", () => {
  const reps = [
    { counts: { moves: 10, due: 2, new: 3 }, gaps: { open: 1, scanned: true, missing: 4, coverage: 0.5 } },
    { counts: { moves: 5, due: 0, new: 5 }, gaps: { open: 2, scanned: false, missing: 0, coverage: 0 } },
    { counts: { moves: 0, due: 0, new: 0 } },
  ];
  assert.deepEqual(listTotals(reps), { reps: 3, moves: 15, holes: 7, due: 2, fresh: 8 });
  assert.deepEqual(listTotals([]), { reps: 0, moves: 0, holes: 0, due: 0, fresh: 0 });
});

test("the openings to start from: both colours, one line each, names of the dictionary", () => {
  assert.equal(new Set(PRESETS.map((p) => p.id)).size, PRESETS.length);
  assert.ok(presetsFor("white").length >= 4 && presetsFor("black").length >= 4);
  for (const p of PRESETS) {
    assert.match(p.pgn, /^1\. \S+( \S+)*( \*)$/);
    assert.ok(p.name.startsWith("openings2.preset."));
    assert.ok(p.color === "white" || p.color === "black");
  }
  assert.equal(presetById("sicilian").color, "black");
  assert.equal(presetById("nope"), null);
});

test("a missing reply the person has just added is the same hole as an unanswered line, with its weight kept", () => {
  const scan = { gaps: [{ ...missing(1, 1, "c7c5", "c5", 0.17), pct: 17, games: 1700 }, missing(1, 1, "c7c6", "c6", 0.08)] };
  const shown = visibleGaps(scan, tree); // c5 is in the tree (move 4) and has no answer
  assert.deepEqual(shown.map(keyOf), ["answer:4:c7c5", "missing:1:c7c6"]);
  assert.equal(shown[0].weight, 0.17);
  assert.equal(shown[0].pct, 17);
  assert.equal(shown[0].ply, 2);
  // no duplicate when the search itself already lists the unanswered line
  const both = visibleGaps({ gaps: [missing(1, 1, "c7c5", "c5", 0.17), answer(4, 2, "c7c5", "c5", 0.17)] }, tree);
  assert.deepEqual(both.map(keyOf), ["answer:4:c7c5"]);
});

test("a hole is one for a position reached by two orders of moves", () => {
  // 1.d4 Nf6 2.c4 e6 (ids 1-4) and 1.c4 e6 2.d4 Nf6 (ids 5-8) end in one position, with no answer to it
  const pos = "same-position";
  const ms = [
    mv(1, 0, "d2d4", "d4", 1, true),
    mv(2, 1, "g8f6", "Nf6", 2, false),
    mv(3, 2, "c2c4", "c4", 3, true),
    { ...mv(4, 3, "e7e6", "e6", 4, false), fen: pos },
    mv(5, 0, "c2c4", "c4", 1, true),
    mv(6, 5, "e7e6", "e6", 2, false),
    mv(7, 6, "d2d4", "d4", 3, true),
    { ...mv(8, 7, "g8f6", "Nf6", 4, false), fen: pos },
  ];
  const t = buildTree(ms);
  const gaps = visibleGaps(null, t);
  assert.deepEqual(gaps.map(keyOf), ["answer:4:e7e6"]);
  // both nodes are marked with the dot
  assert.deepEqual([...markedNodes(gaps, t)].sort(), [4, 8]);
  // the server lists the second node, the page the first: still one hole
  const scan = { gaps: [{ ...answer(8, 4, "g8f6", "Nf6", 0.4) }] };
  assert.equal(visibleGaps(scan, t).length, 1);
  // an answer on either branch closes it
  const answered = buildTree([...ms, mv(9, 8, "b1c3", "Nc3", 5, true)]);
  assert.deepEqual(visibleGaps(scan, answered), []);
  // a popular reply added on the other branch is no longer missing, only an unanswered one
  const m = { kind: "missing", at: 4, ply: 4, fen: "f", path: [], uci: "f1b5", san: "Bb5", pct: 20, games: 99, weight: 0.2, source: "lichess" };
  const withReply = buildTree([...ms, mv(9, 8, "f1b5", "Bb5", 5, false)]);
  assert.deepEqual(visibleGaps({ gaps: [m] }, withReply).map((g) => g.kind), ["answer"]);
});

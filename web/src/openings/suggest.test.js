import { test } from "node:test";
import assert from "node:assert/strict";
import { buildSuggestions, makeQueue } from "./suggest.js";

const theory = [
  { uci: "g1f3", san: "Nf3", lines: 100, opening: { eco: "C44", name: "King's Knight Opening" } },
  { uci: "f1c4", san: "Bc4", lines: 60 },
  { uci: "a2a3", san: "a3", lines: 2 },
  { uci: "d2d4", san: "d4", lines: 80 },
];
const ev = (winPct) => ({ winPct, score: 0 });

test("the repertoire's moves come first, then theory by lines, the engine's own last", () => {
  const children = [{ uci: "f1c4", san: "Bc4", id: 7 }];
  const rows = buildSuggestions({ children, theory, best: { uci: "b1c3", san: "Nc3" }, side: "white" });
  assert.deepEqual(rows.map((r) => r.san), ["Bc4", "Nf3", "d4", "a3", "Nc3"]);
  assert.equal(rows[0].repo.id, 7);
  assert.equal(rows[0].theory, true, "a repertoire move that is also theory keeps both marks");
  assert.equal(rows[4].engine, true);
  assert.equal(rows[1].opening.name, "King's Knight Opening");
});

test("a move of the repertoire that theory does not know is still listed", () => {
  const rows = buildSuggestions({ children: [{ uci: "h2h4", san: "h4" }], theory, side: "white" });
  const h4 = rows.find((r) => r.san === "h4");
  assert.ok(h4.repo && !h4.theory && h4.lines === 0);
});

test("share of the best known move, and rare ones", () => {
  const rows = buildSuggestions({ theory, side: "white" });
  assert.equal(rows[0].share, 1);
  assert.equal(rows.find((r) => r.san === "d4").share, 0.8);
  assert.equal(rows.find((r) => r.san === "a3").rare, true);
  assert.equal(rows.find((r) => r.san === "Bc4").rare, false);
});

test("the loss is the mover's, and weak moves sink to the end of their group", () => {
  const before = ev(55); // White is a little better
  const after = new Map([
    ["g1f3", ev(54)], // loses 1
    ["d2d4", ev(30)], // loses 25 for White: weak
    ["f1c4", ev(55)],
  ]);
  const rows = buildSuggestions({ theory, after, before, side: "white" });
  assert.deepEqual(rows.map((r) => r.san), ["Nf3", "Bc4", "a3", "d4"]); // by lines, the weak d4 last
  const d4 = rows.find((r) => r.san === "d4");
  assert.equal(d4.weak, true);
  assert.equal(d4.loss, 25);
  assert.equal(rows[rows.length - 1].san, "d4", "the weak move is last");
  // the same numbers for Black are a gain for Black: nothing is weak
  const asBlack = buildSuggestions({ theory, after, before, side: "black" });
  assert.equal(asBlack.find((r) => r.san === "d4").weak, false);
  assert.equal(asBlack.find((r) => r.san === "a3").loss, null, "no verdict yet");
});

test("with the statistics of real games, popularity is the share of games", () => {
  const stats = {
    available: true,
    total: 1000,
    moves: [
      { uci: "f1c4", san: "Bc4", games: 300, white: 150, draws: 50, black: 100, averageRating: 1650 },
      { uci: "g1f3", san: "Nf3", games: 650, white: 330, draws: 60, black: 260, averageRating: 1660, opening: { eco: "C44", name: "King's Knight Opening" } },
      { uci: "b1c3", san: "Nc3", games: 5, white: 2, draws: 1, black: 2 },
    ],
  };
  const rows = buildSuggestions({ theory, stats, side: "white" });
  // by games now (Nf3 650, Bc4 300), not by lines of theory (Nf3 100, Bc4 60, d4 80)
  assert.deepEqual(rows.map((r) => r.san), ["Nf3", "Bc4", "Nc3", "d4", "a3"]);
  const nf3 = rows[0];
  assert.equal(nf3.share, 0.65);
  assert.equal(nf3.games, 650);
  assert.equal(nf3.stat && nf3.theory, true);
  assert.equal(nf3.opening.name, "King's Knight Opening", "the theory's name is kept");
  assert.equal(rows.find((r) => r.san === "Nc3").rare, true, "5 games in 1000 is rare");
  assert.equal(rows.find((r) => r.san === "Nc3").theory, false, "played, but outside the reference");
  assert.equal(rows.find((r) => r.san === "Bc4").rare, false);
  // theory the games never reached stays at the end of the known moves
  assert.equal(rows.find((r) => r.san === "d4").games, 0);
});

test("statistics that are off, empty or not yet there change nothing", () => {
  const plain = buildSuggestions({ theory, side: "white" }).map((r) => r.san);
  for (const stats of [null, { available: false, total: 0, moves: [] }, { available: true, total: 0, moves: [] }, { available: true, total: 5, moves: [] }]) {
    assert.deepEqual(buildSuggestions({ theory, stats, side: "white" }).map((r) => r.san), plain);
  }
});

test("rating presets name the explorer's groups", async () => {
  const { RATING_PRESETS, DEFAULT_PRESET } = await import("./suggest.js");
  assert.ok(RATING_PRESETS.some((p) => p.id === DEFAULT_PRESET));
  for (const p of RATING_PRESETS) assert.match(p.ratings, /^\d+(,\d+)*$/);
});

test("junk input does not throw", () => {
  assert.deepEqual(buildSuggestions({ side: "white" }), []);
  assert.deepEqual(buildSuggestions({ children: [], theory: [], best: {}, side: "white" }), []);
});

test("the queue runs a few tasks at once and keeps the order", async () => {
  const run = makeQueue(2);
  let active = 0;
  let peak = 0;
  const order = [];
  const task = (n) => async () => {
    active++;
    peak = Math.max(peak, active);
    await new Promise((r) => setTimeout(r, 15));
    active--;
    order.push(n);
    return n * 10;
  };
  const results = await Promise.all([1, 2, 3, 4, 5].map((n) => run(task(n))));
  assert.deepEqual(results, [10, 20, 30, 40, 50]);
  assert.equal(peak, 2);
  assert.deepEqual(order.slice().sort(), [1, 2, 3, 4, 5]);
});

test("a failing task does not stop the queue", async () => {
  const run = makeQueue(1);
  const bad = run(() => Promise.reject(new Error("boom")));
  const good = run(() => 7);
  await assert.rejects(bad, /boom/);
  assert.equal(await good, 7);
});

test("short numbers and shares for the rows", async () => {
  const { formatGames, formatShare, resultShares } = await import("./suggest.js");
  assert.equal(formatGames(0), "0");
  assert.equal(formatGames(812), "812");
  assert.equal(formatGames(1234), "1,2 тыс.");
  assert.equal(formatGames(340000), "340 тыс.");
  assert.equal(formatGames(1234567), "1,2 млн");
  assert.equal(formatShare(0.42), "42%");
  assert.equal(formatShare(0.004), "<1%");
  assert.equal(formatShare(0), "0%");
  assert.deepEqual(resultShares({ white: 300, draws: 50, black: 150 }), { white: 60, draws: 10, black: 30 });
  assert.deepEqual(resultShares({ white: 1, draws: 1, black: 1 }), { white: 33, draws: 33, black: 34 });
  assert.deepEqual(resultShares({ white: 0, draws: 0, black: 0 }), { white: 0, draws: 0, black: 0 });
});

import { test } from "node:test";
import assert from "node:assert/strict";
import { RETRY_GAP, advance, judge, reportFor, startQueue, summary } from "./session.js";

const drill = (move, isNew = false, extra = {}) => ({ move, new: isNew, answer: { uci: "g1f3" }, alt: ["b1c3"], ...extra });

test("new moves are shown first, the others asked at once", () => {
  const q = startQueue([drill(1, true), drill(2)]);
  assert.deepEqual(q.map((e) => [e.drill.move, e.show]), [[1, true], [2, false]]);
});

test("a move is right, another move of the repertoire, or wrong", () => {
  const d = drill(1);
  assert.equal(judge(d, "g1f3"), "right");
  assert.equal(judge(d, "b1c3"), "alt");
  assert.equal(judge(d, "a2a3"), "wrong");
  assert.equal(judge({ move: 1, answer: { uci: "e2e4" } }, "d2d4"), "wrong"); // no alt list at all
});

test("a right answer removes the drill from the queue", () => {
  const q = advance(startQueue([drill(1), drill(2)]), "right");
  assert.deepEqual(q.map((e) => e.drill.move), [2]);
});

test("a new move that was shown comes back at the end as a question", () => {
  const q = advance(startQueue([drill(1, true), drill(2), drill(3)]), "shown");
  assert.deepEqual(q.map((e) => e.drill.move), [2, 3, 1]);
  const back = q[2];
  assert.equal(back.show, false);
  assert.equal(back.recall, true);
});

test("a failed move returns after a few others, or at the end of a short queue", () => {
  const many = advance(startQueue([drill(1), drill(2), drill(3), drill(4), drill(5), drill(6)]), "wrong");
  assert.deepEqual(many.map((e) => e.drill.move), [2, 3, 4, 1, 5, 6]);
  assert.equal(RETRY_GAP, 3);
  assert.equal(many[3].retry, true);
  const short = advance(startQueue([drill(1), drill(2)]), "wrong");
  assert.deepEqual(short.map((e) => e.drill.move), [2, 1]);
  const alone = advance(startQueue([drill(1)]), "wrong");
  assert.deepEqual(alone.map((e) => e.drill.move), [1]);
  assert.deepEqual(advance([], "right"), []);
});

test("only the first result of a move goes to the server", () => {
  const e = startQueue([drill(1)])[0];
  const reported = new Set();
  assert.equal(reportFor(reported, e, "wrong"), false);
  reported.add(1);
  assert.equal(reportFor(reported, e, "right"), null, "a later answer is not sent again");
  // a new move that was only shown is not reported; its first question is
  const fresh = startQueue([drill(2, true)])[0];
  assert.equal(reportFor(new Set(), fresh, "shown"), null);
  assert.equal(reportFor(new Set(), { ...fresh, show: false, recall: true }, "right"), true);
});

test("the summary counts first results", () => {
  assert.deepEqual(summary({ 1: true, 2: false, 3: true }), { total: 3, right: 2, wrong: 1 });
  assert.deepEqual(summary({}), { total: 0, right: 0, wrong: 0 });
});

test("a deviation is judged against the repertoire's own move, for whoever moves", async () => {
  const { deviationVerdict, deviationText } = await import("./session.js");
  const ev = (winPct) => ({ winPct });
  // White to move: the repertoire's move leaves 55%
  assert.deepEqual(deviationVerdict(ev(55), ev(54), "white"), { kind: "fine", loss: 1 });
  assert.deepEqual(deviationVerdict(ev(55), ev(60), "white"), { kind: "fine", loss: 0 }, "a better move is fine");
  assert.deepEqual(deviationVerdict(ev(55), ev(47), "white"), { kind: "inaccurate", loss: 8 });
  assert.deepEqual(deviationVerdict(ev(55), ev(30), "white"), { kind: "blunder", loss: 25 });
  // Black to move: White's chance rising is Black's loss
  assert.deepEqual(deviationVerdict(ev(45), ev(70), "black"), { kind: "blunder", loss: 25 });
  assert.deepEqual(deviationVerdict(ev(45), ev(40), "black"), { kind: "fine", loss: 0 });
  assert.equal(deviationVerdict(null, ev(40), "white").kind, "unknown");
  assert.equal(deviationVerdict(ev(40), {}, "white").kind, "unknown");

  assert.match(deviationText({ kind: "fine", loss: 1 }, "Nc3", "Nf3"), /хороший, но мы учим другую ветку.*Nf3/);
  assert.match(deviationText({ kind: "inaccurate", loss: 8.4 }, "h3", "Nf3"), /неточный.*8%.*Nf3/);
  assert.match(deviationText({ kind: "blunder", loss: 24.6 }, "Qh5", "Nf3"), /грубая ошибка.*25%.*так играть не надо.*Nf3/);
  assert.match(deviationText({ kind: "unknown", loss: null }, "a3", "Nf3"), /Nf3/);
});

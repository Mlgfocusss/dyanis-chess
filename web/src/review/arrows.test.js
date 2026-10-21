import test from "node:test";
import assert from "node:assert/strict";
import { BASE_OPACITY, BASE_WIDTH, LOSS_SPAN, WIDTH_LOSS, arrowShape, arrowStyle, moverWin, pickArrows, squareCentre } from "./arrows.js";

const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";
const BLACK_TO_MOVE = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1";

test("the best line is the thickest and the brightest, then the second, then the third", () => {
  const a = arrowStyle(0, 0);
  const b = arrowStyle(1, 0);
  const c = arrowStyle(2, 0);
  assert.ok(a.width > b.width && b.width > c.width);
  assert.ok(a.opacity > b.opacity && b.opacity > c.opacity);
  assert.equal(a.width, BASE_WIDTH[0]);
  assert.equal(a.opacity, BASE_OPACITY[0]);
});

test("a line that gives away more of the win is thinner and paler, within limits", () => {
  const same = arrowStyle(1, 0);
  const worse = arrowStyle(1, 15);
  const worst = arrowStyle(1, LOSS_SPAN);
  assert.ok(worse.width < same.width && worst.width < worse.width);
  assert.ok(worse.opacity < same.opacity && worst.opacity < worse.opacity);
  // beyond the span nothing changes, and the arrow never vanishes
  assert.deepEqual(arrowStyle(1, 500), worst);
  assert.ok(worst.width >= BASE_WIDTH[1] * (1 - WIDTH_LOSS) - 1e-9);
  assert.ok(worst.opacity > 0.4);
  // a negative loss (a line better than the best, from a shallower search) counts as none
  assert.deepEqual(arrowStyle(0, -5), arrowStyle(0, 0));
  // even the worst loss of the best line is thicker than the cleanest third line
  assert.ok(arrowStyle(0, LOSS_SPAN).width > arrowStyle(2, 0).width);
  assert.ok(arrowStyle(0, LOSS_SPAN).opacity > arrowStyle(2, 0).opacity);
});

test("moverWin looks from the side to move", () => {
  assert.equal(moverWin(70, "white"), 70);
  assert.equal(moverWin(70, "black"), 30);
});

const lines = [
  { bestUci: "e2e4", winPct: 55 },
  { bestUci: "d2d4", winPct: 53 },
  { bestUci: "g1f3", winPct: 40 },
];

test("three lines make three arrows, the best drawn last (on top)", () => {
  const out = pickArrows({ fen: START, lines });
  assert.deepEqual(out.map((a) => a.uci), ["g1f3", "d2d4", "e2e4"]);
  assert.deepEqual(out.map((a) => a.kind), ["third", "second", "best"]);
  assert.ok(out[2].width > out[1].width && out[1].width > out[0].width);
  assert.equal(out[2].from, "e2");
  assert.equal(out[2].to, "e4");
});

test("the loss is counted from the side to move: for Black a higher White's win is worse", () => {
  const ls = [
    { bestUci: "e7e5", winPct: 45 }, // the best for Black (lowest for White)
    { bestUci: "c7c5", winPct: 60 }, // gives Black 15 points
    { bestUci: "a7a6", winPct: 45 },
  ];
  const out = pickArrows({ fen: BLACK_TO_MOVE, lines: ls });
  const by = Object.fromEntries(out.map((a) => [a.kind, a]));
  assert.equal(by.second.width, arrowStyle(1, 15).width);
  assert.equal(by.third.width, arrowStyle(2, 0).width);
});

test("arrows that start with the same move are drawn once, as the better line", () => {
  const out = pickArrows({ fen: START, lines: [lines[0], { bestUci: "e2e4", winPct: 50 }, lines[2]] });
  assert.deepEqual(out.map((a) => a.uci), ["g1f3", "e2e4"]);
  assert.equal(out[1].kind, "best");
});

test("before the lines come, the review's own best move is the one arrow; with no move, none", () => {
  const out = pickArrows({ fen: START, lines: null, first: { bestUci: "e2e4", winPct: 55 } });
  assert.deepEqual(out.map((a) => [a.uci, a.kind]), [["e2e4", "best"]]);
  assert.deepEqual(pickArrows({ fen: START, lines: null, first: null }), []);
  assert.deepEqual(pickArrows({ fen: START, lines: [], first: {} }), []);
});

test("Dionis's red arrow takes the engine's best move over from the green one", () => {
  const out = pickArrows({ fen: START, lines, first: { bestUci: "e2e4", winPct: 55 }, threat: true });
  assert.deepEqual(out.map((a) => [a.uci, a.kind]), [["g1f3", "third"], ["d2d4", "second"], ["e2e4", "threat"]]);
  assert.equal(out.filter((a) => a.uci === "e2e4").length, 1);
});

test("with the engine arrows off only the red arrow is left", () => {
  assert.deepEqual(pickArrows({ fen: START, lines, first: { bestUci: "e2e4" }, enabled: false }), []);
  const out = pickArrows({ fen: START, lines, first: { bestUci: "e2e4" }, threat: true, enabled: false });
  assert.deepEqual(out.map((a) => a.kind), ["threat"]);
});

test("a line with no win probability is drawn without a loss", () => {
  const out = pickArrows({ fen: START, lines: [{ bestUci: "e2e4" }, { bestUci: "d2d4" }] });
  assert.equal(out[0].width, arrowStyle(1, 0).width);
});

test("squareCentre follows the orientation of the board", () => {
  assert.deepEqual(squareCentre("a8", "white"), [0.5, 0.5]);
  assert.deepEqual(squareCentre("h1", "white"), [7.5, 7.5]);
  assert.deepEqual(squareCentre("a8", "black"), [7.5, 7.5]);
  assert.deepEqual(squareCentre("h1", "black"), [0.5, 0.5]);
});

test("a straight arrow has a shaft ending where the head begins, and the head points at the target", () => {
  const { shaft, head } = arrowShape("e2", "e4", "white", 0.2);
  assert.equal(shaft.length, 2);
  assert.deepEqual(shaft[0], [4.5, 6.5]);
  const [tip, l, r] = head;
  assert.ok(Math.abs(tip[0] - 4.5) < 1e-9 && tip[1] < 4.5 + 0.2 && tip[1] > 4.5); // just short of the centre of e4
  assert.ok(shaft[1][1] > tip[1]); // the shaft ends behind the tip
  assert.ok(Math.abs(l[0] - 4.5) > 0.2 && Math.abs(r[0] - 4.5) > 0.2 && l[0] !== r[0]); // the head is wider than the shaft
});

test("a knight's move bends: the long way first, then the short", () => {
  const w = arrowShape("g1", "f3", "white", 0.15);
  assert.equal(w.shaft.length, 3);
  assert.deepEqual(w.shaft[1], [6.5, 5.5]); // up two squares from g1, then left one
  const h = arrowShape("b1", "c3", "black", 0.15);
  assert.equal(h.shaft.length, 3);
  const flat = arrowShape("b1", "d2", "white", 0.15);
  assert.deepEqual(flat.shaft[1], [3.5, 7.5]); // two across first
});

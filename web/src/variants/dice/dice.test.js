import test from "node:test";
import assert from "node:assert/strict";
import { FACE, NAME, PIPS, describeLost, describeTurn, dieCaption, diceSlots, leftNames, movableSquares, needsRoll, newThrow, pieceIcon, shakeKinds, sideOfDice, usedCount } from "./dice.js";

const three = (...states) => ({
  of: 3,
  dice: [
    { die: "N", state: states[0] },
    { die: "P", state: states[1] },
    { die: "R", state: states[2] },
  ],
  left: states.filter((s) => s === "free").length,
});

test("every face has as many pips as its number, on distinct places", () => {
  for (const n of [1, 2, 3, 4, 5, 6]) {
    assert.equal(PIPS[n].length, n);
    assert.equal(new Set(PIPS[n].map((p) => p.join(","))).size, n);
  }
});

test("the six kinds are the six faces, in the author's order", () => {
  assert.deepEqual(Object.entries(FACE).sort((a, b) => a[1] - b[1]).map(([k]) => k), ["P", "N", "B", "R", "Q", "K"]);
  assert.equal(NAME.N, "конь");
});

test("the pieces that may move are the from-squares of the board moves", () => {
  assert.deepEqual(movableSquares(["e2e4", "e2e3", "g1f3", "e7e8q", "e1g1"]), {
    e2: "dice-movable",
    g1: "dice-movable",
    e7: "dice-movable",
    e1: "dice-movable",
  });
  assert.deepEqual(movableSquares([]), {});
  assert.deepEqual(movableSquares(undefined), {});
});

test("the three slots hold the three dice of the turn", () => {
  const ex = three("free", "free", "skip");
  assert.deepEqual(diceSlots(ex), ex.dice);
  assert.deepEqual(diceSlots(undefined), [null, null, null]);
  assert.deepEqual(diceSlots({ dice: [ex.dice[0]] }), [ex.dice[0], null, null]);
});

test("the roll button shows to the person to move until they press it or play", () => {
  const fresh = three("free", "free", "skip");
  assert.equal(needsRoll(fresh, true, false), true);
  assert.equal(needsRoll(fresh, true, true), false); // pressed
  assert.equal(needsRoll(fresh, false, false), false); // the other side sees the dice at once
  assert.equal(needsRoll(three("used", "free", "skip"), true, false), false); // a move was made: the dice are known
  assert.equal(needsRoll(undefined, true, false), false);
  assert.equal(needsRoll({ dice: [] }, true, false), false);
});

test("used dice are counted and the names of the rest are listed in the order thrown", () => {
  assert.equal(usedCount(three("used", "free", "skip")), 1);
  assert.equal(usedCount(undefined), 0);
  assert.equal(leftNames(three("used", "free", "skip")), "пешка");
  assert.equal(leftNames(three("used", "used", "used")), "");
});

test("the words about the dice depend on who looks", () => {
  const ex = three("free", "free", "skip");
  assert.equal(describeTurn(ex, true, "white"), "Осталось ходов: 2 из 3. Ходите любой из фигур в любом порядке: конь, пешка.");
  assert.equal(describeTurn(ex, false, "black"), "Чёрные: осталось ходов 2 из 3 (конь, пешка).");
  assert.equal(describeTurn(three("used", "free", "skip"), true, "white"), "Осталось ходов: 1 из 3. Ходите любой из фигур в любом порядке: пешка.");
  assert.equal(describeTurn({}, true, "white"), "");
  assert.equal(dieCaption("skip"), "нечем ходить, пропуск");
  assert.equal(dieCaption("used"), "сыграно");
  assert.equal(describeLost(null), "");
  assert.match(describeLost({ side: "black", dice: ["B", "R", "Q"] }), /^Чёрные пропустили очередь: выпали слон, ладья, ферзь/);
});

test("the shaking dice show three kinds", () => {
  const k = shakeKinds(() => 0.99);
  assert.deepEqual(k, ["K", "K", "K"]);
  assert.equal(shakeKinds().length, 3);
  for (const x of shakeKinds()) assert.ok(x in FACE);
});

test("skipped dice are not counted among the ones left", () => {
  const ex = three("free", "skip", "skip");
  assert.equal(leftNames(ex), "конь");
  assert.equal(describeTurn(ex, false, "white"), "Белые: осталось ходов 1 из 3 (конь).");
});

test("an animation plays for a new throw only, never for the first view (a reload)", () => {
  const a = { dice: [{}, {}, {}], turns: 4 };
  let r = newThrow(undefined, a);
  assert.deepEqual(r, { seen: 4, fresh: false }); // first view: remembered, not played
  r = newThrow(r.seen, { ...a }); // an ordinary refresh of the same turn
  assert.equal(r.fresh, false);
  r = newThrow(r.seen, { ...a, turns: 5 }); // the next turn: a new throw
  assert.deepEqual(r, { seen: 5, fresh: true });
  assert.deepEqual(newThrow(5, undefined), { seen: 5, fresh: false });
});

test("every kind has an icon name and the dice wear the colour of the side to move", () => {
  assert.deepEqual(Object.keys(FACE).map(pieceIcon), ["pieceP", "pieceN", "pieceB", "pieceR", "pieceQ", "pieceK"]);
  assert.equal(sideOfDice("black"), "black");
  assert.equal(sideOfDice("white"), "white");
  assert.equal(sideOfDice(undefined), "white");
});

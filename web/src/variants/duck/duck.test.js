import test from "node:test";
import assert from "node:assert/strict";
import { duckClick, duckDots, stepOf } from "./duck.js";

const legalDuck = ["@a3", "@e5", "@h8"];
const base = (over = {}) => ({ myTurn: true, legal: legalDuck, ui: {}, settings: {}, view: { extra: { step: "duck", duck: "c4" } }, ...over });

test("the step is read from the server's extra, a piece move by default", () => {
  assert.equal(stepOf({ view: { extra: { step: "duck" } } }), "duck");
  assert.equal(stepOf({ view: { extra: { step: "move" } } }), "move");
  assert.equal(stepOf({ view: {} }), "move");
  assert.equal(stepOf({}), "move");
});

test("the dots of the duck step are the free squares the server offers", () => {
  assert.deepEqual(duckDots(legalDuck), { a3: "dot", e5: "dot", h8: "dot" });
  assert.deepEqual(duckDots(["e2e4", "g1f3"]), {});
  assert.deepEqual(duckDots(undefined), {});
});

test("a click puts the duck down at once, or marks the square first when moves are confirmed", () => {
  assert.deepEqual(duckClick("e5", base()), { handled: true, act: "@e5" });
  assert.deepEqual(duckClick("e5", base({ settings: { confirmMove: true } })), { handled: true, pending: "e5" });
  assert.deepEqual(duckClick("e5", base({ settings: { confirmMove: true }, ui: { duckPending: "e5" } })), { handled: true, act: "@e5" });
  // another square moves the mark
  assert.deepEqual(duckClick("a3", base({ settings: { confirmMove: true }, ui: { duckPending: "e5" } })), { handled: true, pending: "a3" });
});

test("a click on an occupied square or the duck's own is swallowed, outside the duck step it is not ours", () => {
  assert.deepEqual(duckClick("e2", base()), { handled: true });
  assert.deepEqual(duckClick("c4", base()), { handled: true });
  assert.deepEqual(duckClick("e5", base({ myTurn: false })), { handled: false });
  assert.deepEqual(duckClick("e5", base({ view: { extra: { step: "move" } }, legal: ["e2e4"] })), { handled: false });
});

import test from "node:test";
import assert from "node:assert/strict";
import { countWentUp, freshTurns, openRoomId, titleWithCount } from "./logic.js";

test("titleWithCount puts the number in front and takes it off again", () => {
  assert.equal(titleWithCount("Dyanis Chess", 2), "(2) Dyanis Chess");
  assert.equal(titleWithCount("(2) Dyanis Chess", 3), "(3) Dyanis Chess");
  assert.equal(titleWithCount("(3) Dyanis Chess", 0), "Dyanis Chess");
  assert.equal(titleWithCount("Dyanis Chess", 0), "Dyanis Chess");
  assert.equal(titleWithCount("2048 Chess", 1), "(1) 2048 Chess"); // a number that is part of the title stays
});

test("countWentUp does not announce what was waiting before the first answer", () => {
  assert.equal(countWentUp(null, 3), false);
  assert.equal(countWentUp(undefined, 1), false);
});

test("countWentUp announces only a rise", () => {
  assert.equal(countWentUp(0, 1), true);
  assert.equal(countWentUp(1, 2), true);
  assert.equal(countWentUp(2, 2), false);
  assert.equal(countWentUp(2, 1), false);
});

const g = (id, plies, yourTurn = true) => ({ id, plies, yourTurn, opponent: { name: id } });

test("freshTurns: a game that was not known is fresh", () => {
  const r = freshTurns([g("a", 4), g("b", 3, false)], new Map());
  assert.deepEqual(r.fresh.map((x) => x.id), ["a"]);
  assert.deepEqual([...r.known], [["a", 4]]);
});

test("freshTurns: a game still waiting with the same moves is not fresh again", () => {
  assert.deepEqual(freshTurns([g("a", 4)], new Map([["a", 4]])).fresh, []);
});

test("freshTurns: a game answered by the person and moved again by the opponent is fresh", () => {
  const r = freshTurns([g("a", 6)], new Map([["a", 4]]));
  assert.deepEqual(r.fresh.map((x) => x.id), ["a"]);
  assert.equal(r.known.get("a"), 6);
});

test("freshTurns copes with no list", () => {
  assert.deepEqual(freshTurns(null, new Map()).fresh, []);
});

test("openRoomId reads the id of the open room", () => {
  assert.equal(openRoomId("#online/abc123"), "abc123");
  assert.equal(openRoomId("#online/abc123?x=1"), "abc123");
  assert.equal(openRoomId("#community/correspondence"), "");
  assert.equal(openRoomId(""), "");
});

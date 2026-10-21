import test from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULT_PREFS,
  boardToShow,
  clickResult,
  lastMoveLine,
  lastMoveToShow,
  moveNumber,
  promotionLetters,
  readPrefs,
  sourcesOf,
  targetsOf,
  writePrefs,
} from "./blindfold.js";

const LEGAL = ["e2e4", "e2e3", "g1f3", "g1h3", "e1g1", "P@e4", "-"];

test("only actions from one square to another are clicks", () => {
  assert.deepEqual([...sourcesOf(LEGAL)].sort(), ["e1", "e2", "g1"]);
  assert.deepEqual(targetsOf(LEGAL, "e2").sort(), ["e3", "e4"]);
  assert.deepEqual(targetsOf(LEGAL, "g1").sort(), ["f3", "h3"]);
  assert.deepEqual(targetsOf(LEGAL, "a1"), []);
  assert.deepEqual([...sourcesOf(undefined)], []);
});

test("a promotion offers its pieces, queen first", () => {
  const legal = ["a7a8n", "a7a8b", "a7a8r", "a7a8q", "a7b8q", "a7b8n"];
  assert.deepEqual(promotionLetters(legal, "a7", "a8"), ["q", "r", "b", "n"]);
  assert.deepEqual(targetsOf(legal, "a7").sort(), ["a8", "b8"]);
  assert.equal(promotionLetters(LEGAL, "e2", "e4"), null);
});

test("clicks pick a piece, then its square", () => {
  assert.deepEqual(clickResult(LEGAL, {}, "e2"), { type: "select", from: "e2" });
  assert.deepEqual(clickResult(LEGAL, {}, "d5"), { type: "none", empty: true });
  assert.deepEqual(clickResult(LEGAL, { from: "e2" }, "e4"), { type: "move", action: "e2e4" });
  assert.deepEqual(clickResult(LEGAL, { from: "e1" }, "g1"), { type: "move", action: "e1g1" }); // the castling
  assert.deepEqual(clickResult(LEGAL, { from: "e2" }, "e2"), { type: "clear" });
  assert.deepEqual(clickResult(LEGAL, { from: "e2" }, "g1"), { type: "select", from: "g1" }); // another piece
  assert.deepEqual(clickResult(LEGAL, { from: "e2" }, "h6"), { type: "clear" });
  assert.deepEqual(clickResult([], {}, "e2"), { type: "none", empty: true });
  const promo = ["a7a8q", "a7a8r", "a7a8b", "a7a8n"];
  assert.deepEqual(clickResult(promo, { from: "a7" }, "a8"), { type: "promote", from: "a7", to: "a8", letters: ["q", "r", "b", "n"] });
});

test("the last move is told in words", () => {
  const moves = [
    { side: "white", san: "e4" },
    { side: "black", san: "Nf6" },
  ];
  assert.equal(lastMoveLine(moves, "white"), "Соперник сыграл Nf6");
  assert.equal(lastMoveLine(moves.slice(0, 1), "white"), "Вы сыграли e4. Ждём ответа");
  assert.equal(lastMoveLine(moves, ""), "Чёрные сыграли Nf6");
  assert.equal(lastMoveLine([{ side: "black", san: "Qh4#" }], "white", true), "Соперник сыграл Qh4#: мат");
  assert.equal(lastMoveLine([{ side: "white", san: "Bb5+" }], "black"), "Соперник сыграл Bb5+: шах");
  assert.equal(lastMoveLine([], "white"), "");
  assert.equal(moveNumber(moves), "1...");
  assert.equal(moveNumber(moves.slice(0, 1)), "1.");
  assert.equal(moveNumber([...moves, { side: "white", san: "Nc3" }]), "2.");
  assert.equal(moveNumber([]), "");
});

test("the board shown: nothing while hidden, the position while peeking, everything at the end", () => {
  const position = { e4: "wP" };
  // blind, peeking not allowed: nothing, whoever looks
  for (const you of ["white", "black", ""]) assert.deepEqual(boardToShow({ extra: { blind: true, peek: false }, you, peeking: true }), {});
  // blind, peeking allowed: a player sees it only while peeking; a spectator always
  const extra = { blind: true, peek: true, board: position };
  assert.deepEqual(boardToShow({ extra, you: "white", peeking: false }), {});
  assert.equal(boardToShow({ extra, you: "white", peeking: true }), position);
  assert.equal(boardToShow({ extra, you: "", peeking: false }), position);
  // over: the snapshot's own board is used
  assert.equal(boardToShow({ extra: { blind: false, peek: true }, you: "white", peeking: true }), null);
  assert.equal(boardToShow({ extra: undefined, you: "white" }), null);
});

test("the settings of the room are kept, with safe defaults", () => {
  const store = new Map();
  const storage = { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => store.set(k, v) };
  assert.deepEqual(readPrefs(storage), DEFAULT_PREFS);
  assert.equal(DEFAULT_PREFS.showMoves, false); // the list of moves is hidden by default
  assert.equal(DEFAULT_PREFS.oppLast, true); // the opponent's last move is lit by default
  writePrefs(storage, { showMoves: true, dots: false, oppLast: false });
  assert.deepEqual(readPrefs(storage), { showMoves: true, dots: false, oppLast: false });
  // rubbish, no storage, a storage that throws
  store.set("dyanis.blindfold.v1", "{nope");
  assert.deepEqual(readPrefs(storage), DEFAULT_PREFS);
  store.set("dyanis.blindfold.v1", JSON.stringify({ showMoves: "yes", dots: 0 }));
  assert.deepEqual(readPrefs(storage), DEFAULT_PREFS);
  assert.deepEqual(readPrefs(undefined), DEFAULT_PREFS);
  const broken = {
    getItem() {
      throw new Error("blocked");
    },
    setItem() {
      throw new Error("blocked");
    },
  };
  assert.deepEqual(readPrefs(broken), DEFAULT_PREFS);
  assert.doesNotThrow(() => writePrefs(broken, DEFAULT_PREFS));
});

test("lastMoveToShow: the opponent's move for a player, any move for a spectator and after the end", () => {
  const last = ["e2", "e4"];
  const moves = [{ side: "white", san: "e4" }];
  const on = { last, moves, blind: true, oppLast: true };
  assert.deepEqual(lastMoveToShow({ ...on, you: "black" }), { from: "e2", to: "e4" });
  assert.equal(lastMoveToShow({ ...on, you: "white" }), null); // the own move is not lit
  assert.equal(lastMoveToShow({ ...on, you: "black", oppLast: false }), null);
  assert.deepEqual(lastMoveToShow({ ...on, you: "" }), { from: "e2", to: "e4" });
  assert.deepEqual(lastMoveToShow({ ...on, you: "white", blind: false }), { from: "e2", to: "e4" });
  assert.equal(lastMoveToShow({ ...on, you: "black", last: [] }), null);
  assert.equal(lastMoveToShow({ ...on, you: "black", last: undefined }), null);
});

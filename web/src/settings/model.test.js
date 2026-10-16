import test from "node:test";
import assert from "node:assert/strict";
import { ANIMATION_MS, DEFAULTS, sanitize, GAMEPLAY_KEYS, defaultsOf } from "./model.js";

test("sanitize returns the defaults for junk", () => {
  for (const junk of [null, undefined, 5, "x", [], true]) {
    assert.deepEqual(sanitize(junk), DEFAULTS);
  }
});

test("sanitize keeps valid values and drops invalid ones", () => {
  const out = sanitize({
    boardTheme: "green",
    pieceSet: "cburnett",
    coordinates: "outside",
    highlightLastMove: false,
    moveMethod: "drag",
    animation: "fast",
    sound: false,
    soundTheme: "wood",
    playLayout: "center",
    focusBoard: true,
  });
  assert.deepEqual(out, {
    ...DEFAULTS,
    boardTheme: "green",
    pieceSet: "cburnett",
    background: "none",
    coordinates: "outside",
    highlightLastMove: false,
    moveMethod: "drag",
    animation: "fast",
    sound: false,
    soundTheme: "wood",
    playLayout: "center",
    focusBoard: true,
  });

  const bad = sanitize({
    coordinates: "sideways", // not an allowed choice
    moveMethod: 3, // wrong type
    animation: "warp",
    playLayout: "diagonal",
    focusBoard: "yes", // wrong type
    sound: "yes", // wrong type
    boardTheme: 7,
    unknownKey: "x",
  });
  assert.deepEqual(bad, DEFAULTS);
  assert.ok(!("unknownKey" in bad));
});

test("sanitize does not mutate its input or the defaults", () => {
  const input = { boardTheme: "brown" };
  const before = JSON.stringify(DEFAULTS);
  const out = sanitize(input);
  out.boardTheme = "changed";
  assert.deepEqual(input, { boardTheme: "brown" });
  assert.equal(JSON.stringify(DEFAULTS), before);
});

test("every animation setting has a duration", () => {
  assert.deepEqual(Object.keys(ANIMATION_MS).sort(), ["fast", "medium", "none", "slow"]);
  assert.equal(ANIMATION_MS.none, 0);
  assert.ok(ANIMATION_MS.fast < ANIMATION_MS.medium && ANIMATION_MS.medium < ANIMATION_MS.slow);
});

test("the gameplay settings are checked: a bad choice falls back, numbers are whole and in range", () => {
  const out = sanitize({ castling: "rook", puzzleLevel: "harder", seekDown: 250.6, seekUp: 99999, alwaysQueen: true });
  assert.equal(out.castling, "rook");
  assert.equal(out.puzzleLevel, "harder");
  assert.equal(out.seekDown, 251);
  assert.equal(out.seekUp, 3000);
  assert.equal(out.alwaysQueen, true);
  const bad = sanitize({ castling: "swap", puzzleLevel: "insane", seekDown: -5, seekUp: "10", premoves: 1, confirmGiveUp: "no" });
  assert.equal(bad.castling, DEFAULTS.castling);
  assert.equal(bad.puzzleLevel, DEFAULTS.puzzleLevel);
  assert.equal(bad.seekDown, 0);
  assert.equal(bad.seekUp, 0);
  assert.equal(bad.premoves, DEFAULTS.premoves);
  assert.equal(bad.confirmGiveUp, DEFAULTS.confirmGiveUp);
  assert.equal(sanitize({ seekDown: NaN }).seekDown, 0);
});

test("each page resets only its own settings, and together they are everything", () => {
  const game = defaultsOf(true);
  const board = defaultsOf(false);
  assert.deepEqual(Object.keys(game).sort(), [...GAMEPLAY_KEYS].sort());
  assert.ok(!("boardTheme" in game) && !("castling" in board));
  assert.deepEqual({ ...board, ...game }, DEFAULTS);
  for (const k of GAMEPLAY_KEYS) assert.ok(k in DEFAULTS, k + " is not a setting");
});

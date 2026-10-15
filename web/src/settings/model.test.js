import test from "node:test";
import assert from "node:assert/strict";
import { ANIMATION_MS, DEFAULTS, sanitize } from "./model.js";

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
  });
  assert.deepEqual(out, {
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
  });

  const bad = sanitize({
    coordinates: "sideways", // not an allowed choice
    moveMethod: 3, // wrong type
    animation: "warp",
    playLayout: "diagonal",
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

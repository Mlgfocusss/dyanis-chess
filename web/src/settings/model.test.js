import test from "node:test";
import assert from "node:assert/strict";
import { ANIMATION_MS, DEFAULTS, sanitize, GAMEPLAY_KEYS, INTERFACE_KEYS, defaultsOf, interfaceAttrs, applyInterface } from "./model.js";

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
  const ui = defaultsOf("interface");
  assert.deepEqual(Object.keys(game).sort(), [...GAMEPLAY_KEYS].sort());
  assert.deepEqual(Object.keys(ui).sort(), [...INTERFACE_KEYS].sort());
  assert.ok(!("boardTheme" in game) && !("castling" in board) && !("textSize" in board) && !("textSize" in game));
  assert.ok(!("castling" in ui) && !("boardTheme" in ui));
  assert.deepEqual({ ...board, ...game, ...ui }, DEFAULTS);
  assert.equal(Object.keys(board).length + Object.keys(game).length + Object.keys(ui).length, Object.keys(DEFAULTS).length);
  for (const k of GAMEPLAY_KEYS) assert.ok(k in DEFAULTS, k + " is not a setting");
  for (const k of INTERFACE_KEYS) assert.ok(k in DEFAULTS, k + " is not a setting");
  assert.ok(!GAMEPLAY_KEYS.some((k) => INTERFACE_KEYS.includes(k)), "a setting is on two pages");
});

test("the interface settings are checked and have safe defaults", () => {
  assert.equal(DEFAULTS.theme, "dark");
  assert.equal(DEFAULTS.textSize, "normal");
  assert.equal(DEFAULTS.reduceMotion, false);
  assert.equal(DEFAULTS.highContrast, false);
  const out = sanitize({ textSize: "large", reduceMotion: true, highContrast: true });
  assert.deepEqual([out.textSize, out.reduceMotion, out.highContrast], ["large", true, true]);
  const bad = sanitize({ textSize: "huge", reduceMotion: "yes", highContrast: 1 });
  assert.deepEqual([bad.textSize, bad.reduceMotion, bad.highContrast], ["normal", false, false]);
  // an account saved before these settings existed gets the defaults
  const old = sanitize({ boardTheme: "green" });
  assert.deepEqual([old.textSize, old.reduceMotion, old.highContrast], ["normal", false, false]);
  assert.equal(old.theme, "dark");
});

test("the theme is one of dark, light and system; anything else is dark", () => {
  for (const t of ["dark", "light", "system"]) assert.equal(sanitize({ theme: t }).theme, t);
  for (const bad of ["Light", "auto", "", 1, true, null, ["light"]]) assert.equal(sanitize({ theme: bad }).theme, "dark", String(bad));
});

test("the theme goes to <html> as data-theme: only light and system need the attribute", () => {
  assert.equal(interfaceAttrs({ theme: "dark" })["data-theme"], null);
  assert.equal(interfaceAttrs({ theme: "light" })["data-theme"], "light");
  assert.equal(interfaceAttrs({ theme: "system" })["data-theme"], "system");
  assert.equal(interfaceAttrs({})["data-theme"], null);
  const attrs = new Map();
  const el = { setAttribute: (k, v) => attrs.set(k, v), removeAttribute: (k) => attrs.delete(k) };
  applyInterface(el, { ...DEFAULTS, theme: "light" });
  assert.equal(attrs.get("data-theme"), "light");
  applyInterface(el, { ...DEFAULTS, theme: "system" });
  assert.equal(attrs.get("data-theme"), "system");
  applyInterface(el, DEFAULTS); // back to dark: the attribute goes
  assert.ok(!attrs.has("data-theme"));
});

test("resetting the interface page brings back the dark theme and does not touch the other pages", () => {
  const ui = defaultsOf("interface");
  assert.equal(ui.theme, "dark");
  assert.ok(INTERFACE_KEYS.includes("theme"));
  assert.ok(!("theme" in defaultsOf(true)) && !("theme" in defaultsOf(false)));
  // a saved light theme is replaced by the reset, the rest of the settings stay
  const merged = { ...sanitize({ theme: "light", boardTheme: "green" }), ...ui };
  assert.equal(merged.theme, "dark");
  assert.equal(merged.boardTheme, "green");
});

test("the interface settings come down to attributes of the root element", () => {
  assert.deepEqual(interfaceAttrs(DEFAULTS), { "data-theme": null, "data-text-size": null, "data-reduce-motion": null, "data-contrast": null });
  assert.deepEqual(interfaceAttrs({ textSize: "large", reduceMotion: true, highContrast: true }), {
    "data-theme": null,
    "data-text-size": "large",
    "data-reduce-motion": "on",
    "data-contrast": "high",
  });
  // applyInterface sets and removes them (a stand-in element, no DOM needed)
  const attrs = new Map();
  const el = { setAttribute: (k, v) => attrs.set(k, v), removeAttribute: (k) => attrs.delete(k) };
  applyInterface(el, { textSize: "large", reduceMotion: true, highContrast: false });
  assert.deepEqual([...attrs].sort(), [["data-reduce-motion", "on"], ["data-text-size", "large"]]);
  applyInterface(el, DEFAULTS);
  assert.equal(attrs.size, 0);
});

test("the puzzle points mode is off by default, belongs to the gameplay page and is a plain switch", () => {
  assert.equal(DEFAULTS.puzzlePoints, false);
  assert.ok(GAMEPLAY_KEYS.includes("puzzlePoints"));
  assert.ok("puzzlePoints" in defaultsOf(true) && !("puzzlePoints" in defaultsOf(false)) && !("puzzlePoints" in defaultsOf("interface")));
  assert.equal(sanitize({ puzzlePoints: true }).puzzlePoints, true);
  assert.equal(sanitize({ puzzlePoints: "yes" }).puzzlePoints, false);
});

test("the engine's arrows are on for everybody: settings saved before the option existed get it, and a saved off stays off", () => {
  assert.equal(DEFAULTS.engineArrows, true);
  assert.ok(GAMEPLAY_KEYS.includes("engineArrows"));
  const old = sanitize({ showEval: false, showCoach: true, moveMethod: "click" }); // no engineArrows key
  assert.equal(old.engineArrows, true);
  assert.equal(sanitize(null).engineArrows, true);
  assert.equal(sanitize({ engineArrows: false }).engineArrows, false);
  assert.equal(sanitize({ engineArrows: "no" }).engineArrows, true); // a wrong kind of value is ignored
  assert.equal(defaultsOf(true).engineArrows, true);
});

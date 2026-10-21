import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { AVATAR_PRESETS, avatarChoice, choiceToValue, parseAvatar, presetGradient, presetValue } from "./avatarPresets.js";

test("twelve presets with distinct ids, icons, labels and hues", () => {
  assert.equal(AVATAR_PRESETS.length, 12);
  for (const key of ["id", "icon", "label", "hue"]) {
    assert.equal(new Set(AVATAR_PRESETS.map((p) => p[key])).size, 12, `${key} repeats`);
  }
});

test("every preset icon exists in the Icon set", () => {
  const src = readFileSync(new URL("../ui/Icon.jsx", import.meta.url), "utf8");
  for (const p of AVATAR_PRESETS) {
    assert.match(src, new RegExp(`^  ${p.icon}:`, "m"), `no icon ${p.icon}`);
  }
});

test("the list equals the ids the server accepts (backend/internal/auth/fields.go)", () => {
  const src = readFileSync(new URL("../../../backend/internal/auth/fields.go", import.meta.url), "utf8");
  const m = src.match(/var avatarPresets = \[\]string\{([^}]*)\}/);
  assert.ok(m, "avatarPresets not found in fields.go");
  const server = [...m[1].matchAll(/"([a-z]+)"/g)].map((x) => x[1]);
  assert.deepEqual(AVATAR_PRESETS.map((p) => p.id).sort(), server.sort());
});

test("parseAvatar", () => {
  assert.deepEqual(parseAvatar(""), { kind: "letter" });
  assert.deepEqual(parseAvatar(undefined), { kind: "letter" });
  assert.deepEqual(parseAvatar(null), { kind: "letter" });
  assert.deepEqual(parseAvatar(5), { kind: "letter" });
  const crown = parseAvatar("preset:crown");
  assert.equal(crown.kind, "preset");
  assert.equal(crown.preset.id, "crown");
  assert.deepEqual(parseAvatar("https://lh3.googleusercontent.com/a/x"), { kind: "photo", url: "https://lh3.googleusercontent.com/a/x" });
  assert.deepEqual(parseAvatar("http://x.test/a.png"), { kind: "photo", url: "http://x.test/a.png" });
  // anything else is the letter, never a picture to load
  for (const v of ["preset:", "preset:nope", "Preset:crown", "javascript:alert(1)", "data:image/png;base64,AA", "//x.test/a.png", "crown"]) {
    assert.deepEqual(parseAvatar(v), { kind: "letter" }, v);
  }
});

test("avatarChoice and choiceToValue", () => {
  assert.equal(avatarChoice({ avatarUrl: "" }), "");
  assert.equal(avatarChoice({}), "");
  assert.equal(avatarChoice(null), "");
  assert.equal(avatarChoice({ avatarUrl: "preset:bolt" }), "preset:bolt");
  assert.equal(avatarChoice({ avatarUrl: "preset:gone" }), "");
  assert.equal(avatarChoice({ avatarUrl: "https://x.test/p.png" }), "google");
  assert.equal(choiceToValue("", "https://x.test/p.png"), "");
  assert.equal(choiceToValue("google", "https://x.test/p.png"), "https://x.test/p.png");
  assert.equal(choiceToValue("google", undefined), "");
  assert.equal(choiceToValue("preset:duel", ""), "preset:duel");
  assert.equal(choiceToValue("preset:gone", ""), "");
  assert.equal(presetValue("star"), "preset:star");
});

test("gradients differ and are made of hsl colours", () => {
  const all = AVATAR_PRESETS.map(presetGradient);
  assert.equal(new Set(all).size, 12);
  for (const g of all) assert.match(g, /^linear-gradient\(145deg, hsl\(\d+ 55% 52%\), hsl\(\d+ 60% 34%\)\)$/);
});

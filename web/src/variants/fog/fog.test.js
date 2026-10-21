import test from "node:test";
import assert from "node:assert/strict";
import { FOG_CLASS, fogClasses } from "./fogview.js";

test("the squares in the fog get the veil, the rest nothing", () => {
  assert.deepEqual(fogClasses({ extra: { fog: ["a8", "h1"] } }), { a8: FOG_CLASS, h1: FOG_CLASS });
  assert.deepEqual(fogClasses({ extra: {} }), {});
  assert.deepEqual(fogClasses({}), {});
  assert.deepEqual(fogClasses(undefined), {});
  assert.deepEqual(fogClasses({ extra: { fog: "a8" } }), {});
});

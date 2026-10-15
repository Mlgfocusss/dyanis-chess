import test from "node:test";
import assert from "node:assert/strict";
import { soundKindForSan } from "./kinds.js";

test("sound kind follows the SAN of the move", () => {
  const cases = [
    ["e4", "move"],
    ["Nf3", "move"],
    ["exd5", "capture"],
    ["Qxf7", "capture"],
    ["Bb5+", "check"],
    ["Qxf7+", "check"], // a check outranks a capture
    ["Qh4#", "end"],
    ["O-O", "castle"],
    ["O-O-O", "castle"],
    ["O-O+", "check"],
    ["e8=Q", "promote"],
    ["exd8=Q", "promote"],
    ["e8=Q#", "end"],
  ];
  for (const [san, kind] of cases) assert.equal(soundKindForSan(san), kind, san);
});

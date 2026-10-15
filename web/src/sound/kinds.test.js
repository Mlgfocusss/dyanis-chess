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

import { mapSoundFiles } from "./kinds.js";

test("mapSoundFiles understands lichess and chess.com naming", () => {
  const lichess = mapSoundFiles([
    { name: "Move", ext: "ogg", url: "m.ogg" },
    { name: "Move", ext: "mp3", url: "m.mp3" },
    { name: "Capture", ext: "mp3", url: "c.mp3" },
    { name: "Check", ext: "mp3", url: "k.mp3" },
    { name: "Checkmate", ext: "mp3", url: "e.mp3" },
    { name: "Castles", ext: "mp3", url: "o.mp3" },
    { name: "CountDown1", ext: "mp3", url: "x.mp3" },
  ]);
  assert.deepEqual(lichess, { move: "m.mp3", capture: "c.mp3", check: "k.mp3", castle: "o.mp3", end: "e.mp3" });

  const chesscom = mapSoundFiles([
    { name: "move-self", ext: "mp3", url: "a" },
    { name: "move-check", ext: "mp3", url: "b" },
    { name: "capture", ext: "mp3", url: "c" },
    { name: "castle", ext: "mp3", url: "d" },
    { name: "promote", ext: "mp3", url: "e" },
    { name: "game-end", ext: "mp3", url: "f" },
    { name: "game-start", ext: "mp3", url: "g" },
    { name: "illegal", ext: "mp3", url: "h" },
  ]);
  assert.deepEqual(chesscom, { move: "a", check: "b", capture: "c", castle: "d", promote: "e", end: "f", start: "g" });
});

test("mapSoundFiles rejects folders without a move sound", () => {
  assert.equal(mapSoundFiles([{ name: "Error", ext: "mp3", url: "x" }]), null);
  assert.equal(mapSoundFiles([]), null);
});

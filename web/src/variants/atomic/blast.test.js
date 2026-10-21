import test from "node:test";
import assert from "node:assert/strict";
import { blastClasses, blastOf, describeBlast, pieceName } from "./blast.js";

const extra = { blast: { center: "d4", squares: ["e3", "d4", "c5"], pieces: { e3: "wB", d4: "bP", c5: "bN" } } };

test("the blast is read from the extra, and its absence is null", () => {
  assert.equal(blastOf(undefined), null);
  assert.equal(blastOf(null), null);
  assert.equal(blastOf({}), null);
  assert.equal(blastOf({ blast: { center: "d4", squares: [] } }), null);
  assert.equal(blastOf(extra).center, "d4");
});

test("the squares of the blast are tinted, the centre more", () => {
  assert.deepEqual(blastClasses(blastOf(extra)), {
    e3: "sq-blast",
    d4: "sq-blast sq-blast--center",
    c5: "sq-blast",
  });
  assert.equal(blastClasses(null), undefined);
});

test("pieces are named in Russian with the right gender", () => {
  assert.equal(pieceName("wN"), "белый конь");
  assert.equal(pieceName("bP"), "чёрная пешка");
  assert.equal(pieceName("wR"), "белая ладья");
  assert.equal(pieceName("bQ"), "чёрный ферзь");
  assert.equal(pieceName("xD"), "");
  assert.equal(pieceName(undefined), "");
});

test("the card lists the centre first and counts each side", () => {
  const d = describeBlast(blastOf(extra));
  assert.equal(d.center, "d4");
  assert.deepEqual(d.lost.map((l) => l.square), ["d4", "c5", "e3"]);
  assert.equal(d.lost[0].name, "чёрная пешка");
  assert.equal(d.white, 1);
  assert.equal(d.black, 2);
  assert.equal(describeBlast(null), null);
});

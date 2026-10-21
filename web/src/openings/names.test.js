import test from "node:test";
import assert from "node:assert/strict";
import { openingName, openingTitle, summaryOpening } from "./names.js";

const ru = { eco: "C57", name: "Italian Game: Two Knights Defense", nameRu: "Итальянская партия: защита двух коней" };

test("openingName shows Russian, falls back to English", () => {
  assert.equal(openingName(ru), "Итальянская партия: защита двух коней");
  assert.equal(openingName({ name: "Italian Game" }), "Italian Game");
  assert.equal(openingName(null), "");
  assert.equal(openingName(undefined), "");
});

test("openingTitle holds the English name only when it differs", () => {
  assert.equal(openingTitle(ru), "Italian Game: Two Knights Defense");
  assert.equal(openingTitle({ name: "Italian Game" }), undefined);
  assert.equal(openingTitle({ name: "Same", nameRu: "Same" }), undefined);
  assert.equal(openingTitle(null), undefined);
});

test("summaryOpening handles old summaries without openingRu", () => {
  assert.deepEqual(summaryOpening({ opening: "Italian Game", openingRu: "Итальянская партия" }), { text: "Итальянская партия", title: "Italian Game" });
  assert.deepEqual(summaryOpening({ opening: "Italian Game" }), { text: "Italian Game", title: undefined });
  assert.deepEqual(summaryOpening({}), { text: "", title: undefined });
  assert.deepEqual(summaryOpening(null), { text: "", title: undefined });
});

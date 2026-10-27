import test from "node:test";
import assert from "node:assert/strict";
import { RECENT_KEY, RECENT_MAX, addRecent, readRecent, saveRecent } from "./recent.js";

const e = (id, extra = {}) => ({ id, mode: "rated", ok: true, rating: 1500, at: 1, ...extra });

test("the newest puzzle comes first and a puzzle is listed once", () => {
  let l = [];
  l = addRecent(l, e("a"));
  l = addRecent(l, e("b"));
  l = addRecent(l, e("a", { ok: false }));
  assert.deepEqual(l.map((x) => x.id), ["a", "b"]);
  assert.equal(l[0].ok, false);
});

test("only the last ten are kept", () => {
  let l = [];
  for (let i = 0; i < 15; i++) l = addRecent(l, e("p" + i));
  assert.equal(l.length, RECENT_MAX);
  assert.equal(l[0].id, "p14");
  assert.equal(l[9].id, "p5");
});

test("a broken entry is ignored and a wrong mode falls back to rated", () => {
  assert.deepEqual(addRecent([], { id: "" }), []);
  assert.deepEqual(addRecent([], null), []);
  assert.equal(addRecent([], e("x", { mode: "weird" }))[0].mode, "rated");
});

test("the list is read back from the storage, and a damaged one reads as empty", () => {
  const data = {};
  const storage = { getItem: (k) => data[k] ?? null, setItem: (k, v) => (data[k] = v) };
  saveRecent(storage, [e("a"), e("b")]);
  assert.deepEqual(readRecent(storage).map((x) => x.id), ["a", "b"]);
  data[RECENT_KEY] = "{not json";
  assert.deepEqual(readRecent(storage), []);
  data[RECENT_KEY] = JSON.stringify({ id: "a" });
  assert.deepEqual(readRecent(storage), []);
  data[RECENT_KEY] = JSON.stringify([{ id: 5 }, e("ok")]);
  assert.deepEqual(readRecent(storage).map((x) => x.id), ["ok"]);
  assert.deepEqual(readRecent({ getItem() { throw new Error("blocked"); } }), []);
  saveRecent({ setItem() { throw new Error("blocked"); } }, [e("a")]); // does not throw
});

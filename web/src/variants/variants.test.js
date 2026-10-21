import test from "node:test";
import assert from "node:assert/strict";
import { buildRegistry, folderOf, normalizeVariant } from "./registry-core.js";
import { BASE_REASONS, describeEnd, groupTurns, isBoardMove, lastMoveOf, listRows, mergeClasses } from "./describe.js";

const def = (id, extra = {}) => ({ id, name: `Вид ${id}`, short: "Коротко.", ...extra });

test("the folder of a definition is read from its path", () => {
  assert.equal(folderOf("./atomic/index.js"), "atomic");
  assert.equal(folderOf("./chess960/index.jsx"), "chess960");
  assert.equal(folderOf("./a/b/index.js"), "");
  assert.equal(folderOf("./atomic/other.js"), "");
});

test("a definition gets its defaults and its mistakes are named", () => {
  const ok = normalizeVariant(def("atomic"), "atomic");
  assert.deepEqual(ok.problems, []);
  assert.equal(ok.variant.icon, "variants");
  assert.equal(ok.variant.hidden, false);
  assert.equal(ok.variant.order, 100);
  assert.deepEqual(ok.variant.rules, []);

  assert.match(normalizeVariant(def("atom"), "atomic").problems[0], /не совпадает с именем папки/);
  assert.match(normalizeVariant(def("At_om"), "At_om").problems[0], /латинских букв/);
  assert.match(normalizeVariant({ id: "x", short: "s" }, "x").problems[0], /name/);
  assert.match(normalizeVariant({ id: "x", name: "n" }, "x").problems[0], /short/);
  assert.match(normalizeVariant(def("x", { rules: "no" }), "x").problems[0], /rules/);
  assert.match(normalizeVariant(undefined, "x").problems[0], /по умолчанию/);
  assert.equal(normalizeVariant(def("atom"), "atomic").variant, null);
});

test("the registry sorts, skips the broken and says why", () => {
  const { variants, problems } = buildRegistry([
    { path: "./duck/index.jsx", def: def("duck", { order: 20 }) },
    { path: "./atomic/index.js", def: def("atomic", { order: 10, name: "Атомные" }) },
    { path: "./horde/index.js", def: def("horde", { order: 10, name: "Орда" }) },
    { path: "./broken/index.js", def: { id: "broken" } },
    { path: "./nested/deeper/index.js", def: def("deeper") },
    { path: "./classic/index.js", def: def("classic", { hidden: true, order: 0 }) },
  ]);
  assert.deepEqual(variants.map((v) => v.id), ["classic", "atomic", "horde", "duck"]);
  assert.equal(problems.length, 3);
  assert.ok(problems.some((p) => p.includes("broken")));
  assert.ok(problems.some((p) => p.includes("nested/deeper")));
});

test("the end of a game is described with the variant's own reasons", () => {
  const you = "white";
  assert.equal(describeEnd({ result: "1-0", termination: "king-captured", you }).subtitle, "Белые победили взятием короля");
  const hill = describeEnd({ result: "0-1", termination: "king-of-the-hill", you, reasons: { "king-of-the-hill": ["Царь горы", "королём на горе"] } });
  assert.equal(hill.reason, "Царь горы");
  assert.equal(hill.title, "Вы проиграли");
  assert.equal(hill.subtitle, "Чёрные победили королём на горе");
  assert.equal(describeEnd({ result: "1/2-1/2", termination: "agreement", you: "" }).title, "Ничья");
  assert.equal(describeEnd({ result: "1-0", termination: "resign", you: "" }).title, "Белые победили");
  // an ending nobody described still shows its key rather than nothing
  assert.equal(describeEnd({ result: "1-0", termination: "exploded", you }).reason, "exploded");
  for (const k of Object.keys(BASE_REASONS)) assert.equal(BASE_REASONS[k].length, 2);
});

test("the actions of one side in a row make one turn, and turns make rows of the list", () => {
  const w = (a) => ({ side: "white", action: a, san: a });
  const b = (a) => ({ side: "black", action: a, san: a });
  const turns = groupTurns([w("e4"), w("@e5"), b("e5"), b("@d4"), w("Nf3")]);
  assert.deepEqual(turns.map((t) => [t.side, t.items.length]), [["white", 2], ["black", 2], ["white", 1]]);
  const rows = listRows(turns);
  assert.equal(rows.length, 2);
  assert.equal(rows[0][0].items[0].san, "e4");
  assert.equal(rows[0][1].side, "black");
  assert.equal(rows[1][1], null);
  assert.deepEqual(listRows(groupTurns([])), []);
  assert.deepEqual(listRows(groupTurns([b("e5")])).map((r) => [r[0], r[1].side]), [[null, "black"]]);
});

test("which actions the board makes by itself", () => {
  for (const a of ["e2e4", "e7e8q", "e1g1", "e1h1"]) assert.ok(isBoardMove(a), a);
  for (const a of ["P@e4", "@e5", "-", "e2e9", "", "e7e8k"]) assert.ok(!isBoardMove(a), a);
});

test("class names of two sources meet on a square; the last action as a move", () => {
  assert.deepEqual(mergeClasses({ e1: "sq-check", a1: "x" }, { e1: "fog", h8: "y" }), { e1: "sq-check fog", a1: "x", h8: "y" });
  assert.deepEqual(mergeClasses(undefined, undefined), {});
  assert.deepEqual(lastMoveOf(["e2", "e4"]), { from: "e2", to: "e4" });
  assert.deepEqual(lastMoveOf(["e4"]), { from: "e4", to: "e4" });
  assert.equal(lastMoveOf([]), null);
  assert.equal(lastMoveOf(undefined), null);
});

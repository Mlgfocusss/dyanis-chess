import { test } from "node:test";
import assert from "node:assert/strict";
import { importSummary, nameFromFile, plural } from "./pgnImport.js";

test("plural follows the Russian rules", () => {
  assert.equal(plural(1, "ход", "хода", "ходов"), "1 ход");
  assert.equal(plural(2, "ход", "хода", "ходов"), "2 хода");
  assert.equal(plural(5, "ход", "хода", "ходов"), "5 ходов");
  assert.equal(plural(11, "ход", "хода", "ходов"), "11 ходов");
  assert.equal(plural(21, "ход", "хода", "ходов"), "21 ход");
  assert.equal(plural(112, "ход", "хода", "ходов"), "112 ходов");
  assert.equal(plural(0, "ход", "хода", "ходов"), "0 ходов");
});

test("importSummary tells what was added and what was left out", () => {
  assert.equal(importSummary({ games: 3, skipped: 0, added: 120, moves: 120 }), "Добавлено 120 ходов из файла (3 партии).");
  assert.match(importSummary({ games: 1, skipped: 2, added: 1, moves: 5 }), /^Добавлено 1 ход из файла \(1 партия\)\. 2 партии пропущены: /);
  assert.match(importSummary({ games: 1, skipped: 0, added: 0, moves: 5 }), /^Новых ходов нет/);
  assert.match(importSummary({ games: 2, skipped: 1, added: 4, moves: 9 }), /1 партия пропущена/);
});

test("nameFromFile drops the extension", () => {
  assert.equal(nameFromFile("Italian_Game.pgn"), "Italian Game");
  assert.equal(nameFromFile("my.repertoire.v2.pgn"), "my.repertoire.v2");
  assert.equal(nameFromFile(""), "");
  assert.equal(nameFromFile("x".repeat(200) + ".pgn").length, 80);
});

test("importSummary mentions the moves that were already there by another order of moves", () => {
  assert.match(importSummary({ games: 2, skipped: 0, added: 4, moves: 9, transposed: 1 }), /1 ход из файла уже был в репозитории при другом порядке ходов/);
  assert.match(importSummary({ games: 2, skipped: 0, added: 4, moves: 9, transposed: 3 }), /3 хода из файла уже были/);
  assert.equal(importSummary({ games: 3, skipped: 0, added: 120, moves: 120, transposed: 0 }), "Добавлено 120 ходов из файла (3 партии).");
});

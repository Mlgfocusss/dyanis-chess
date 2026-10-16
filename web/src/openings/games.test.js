import { test } from "node:test";
import assert from "node:assert/strict";
import { deviationLine, outcomeOf } from "./games.js";

test("the result is told from the person's side", () => {
  assert.deepEqual(outcomeOf("1-0", "white"), { kind: "win", text: "Победа" });
  assert.deepEqual(outcomeOf("1-0", "black"), { kind: "loss", text: "Поражение" });
  assert.deepEqual(outcomeOf("0-1", "black"), { kind: "win", text: "Победа" });
  assert.deepEqual(outcomeOf("1/2-1/2", "white"), { kind: "draw", text: "Ничья" });
  assert.equal(outcomeOf("", "white").kind, "open");
});

test("a forgotten move is named with the move that was expected", () => {
  const d = {
    status: "you-deviated",
    ply: 3,
    line: ["e4", "e5"],
    played: { san: "Nc3" },
    expected: [{ san: "Nf3", mine: true }],
  };
  const t = deviationLine(d);
  assert.equal(t.headline, "Вы отклонились на ходу 2. Nc3: в репозитории здесь Nf3.");
  assert.equal(t.line, "1. e4 e5 2. [Nc3]");
});

test("the opponent leaving the preparation, for Black's side of the move number", () => {
  const d = { status: "opponent-deviated", ply: 2, line: ["e4"], played: { san: "e6" }, expected: [{ san: "e5" }, { san: "c5" }] };
  const t = deviationLine(d);
  assert.match(t.headline, /Соперник вышел из вашей подготовки на ходу 1\.\.\. e6\./);
  assert.match(t.headline, /Вы ждали: e5, c5\./);
  assert.match(t.headline, /Ответа на этот ход в репозитории нет/);
  assert.equal(t.line, "1. e4 [e6]");
});

test("the end of a line, a whole game in the book, and the rest", () => {
  const ended = deviationLine({ status: "book-ended", ply: 5, line: ["e4", "e5", "Nf3", "Nc6"], played: { san: "Bc4" }, expected: [] });
  assert.match(ended.headline, /Линия репозитория закончилась на ходу 3\. Bc4/);
  assert.equal(ended.line, "1. e4 e5 2. Nf3 Nc6 3. [Bc4]");
  assert.match(deviationLine({ status: "in-book", ply: 0, line: ["e4", "e5"], expected: [] }).headline, /Вся партия сыграна/);
  assert.match(deviationLine({ status: "empty", ply: 0, line: [], expected: [] }).headline, /пока нет ходов/);
  assert.match(deviationLine({ status: "broken", ply: 0, line: [], expected: [] }).headline, /не удалось/);
});

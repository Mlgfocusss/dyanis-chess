import { test } from "node:test";
import assert from "node:assert/strict";
import { GREETING, brief, isCostly, pieceAt, takes, targetSquare } from "./dionis.js";

const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

test("pieces on squares of a FEN", () => {
  assert.equal(pieceAt(START, "e1"), "k");
  assert.equal(pieceAt(START, "d8"), "q");
  assert.equal(pieceAt(START, "e2"), "p");
  assert.equal(pieceAt(START, "e4"), "");
  assert.equal(pieceAt("8/8/8/8/8/8/8/R3K3 w - - 0 1", "a1"), "r");
  assert.equal(pieceAt("8/8/8/8/8/8/8/R3K3 w - - 0 1", "e1"), "k");
  assert.equal(pieceAt(START, "z9"), "");
  assert.equal(pieceAt("", "e4"), "");
});

test("where a move goes and what it takes", () => {
  assert.equal(targetSquare("Qxg5+"), "g5");
  assert.equal(targetSquare("exd6"), "d6");
  assert.equal(targetSquare("e8=Q#"), "e8");
  assert.equal(targetSquare("O-O"), null);
  const fen = "4k3/8/8/6n1/8/8/8/4K2Q w - - 0 1"; // a knight on g5, a queen on h1
  assert.equal(takes("Qxg5", fen), "коня");
  assert.equal(takes("Qg2", fen), "");
  assert.equal(takes("exd6", "4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 1"), "пешку"); // en passant: the square is empty
});

const mv = (o) => ({ ply: 13, color: "white", san: "Ng5", class: "mistake", bestSan: "Ng1", ...o });
const afterTakingKnight = { fen: "4k3/8/8/6q1/8/8/8/4K3 b - - 0 1", bestSan: "Qxg5" };

test("a mistake says what the reply takes and what to play instead", () => {
  const afterFen = "4k3/8/8/6N1/8/8/8/4K2q b - - 0 1"; // a knight on g5 for the reply to take
  const b = brief({ move: mv(), before: { fen: START }, after: { fen: afterFen, bestSan: "Qxg5" } });
  assert.match(b.text, /^Ошибка: Qxg5 берёт коня\. Нужно было Ng1\.$/);
  assert.equal(b.threat, true);
  assert.equal(b.best, "Ng1");
});

test("a blunder that allows a mate names it, whoever moved", () => {
  const white = brief({ move: mv({ class: "blunder", san: "g4", bestSan: "d4" }), after: { fen: START, bestSan: "Qh4#", mate: -1 } });
  assert.match(white.text, /^Зевок! Позволяет мат в 1\. Нужно было d4\.$/);
  const black = brief({ move: mv({ color: "black", class: "blunder", san: "f6", bestSan: "e6" }), after: { fen: START, bestSan: "Qh5#", mate: 1 } });
  assert.match(black.text, /мат в 1/);
  // a mate for the mover is not something the move allows
  const fine = brief({ move: mv({ class: "mistake" }), after: { fen: START, bestSan: "Qh5", mate: 2 } });
  assert.doesNotMatch(fine.text, /мат/);
});

test("a costly move with nothing to name still says it was costly", () => {
  const b = brief({ move: mv({ class: "inaccuracy" }), after: { fen: START, bestSan: "Nf3" } });
  assert.equal(b.text, "Неточность. Лучше было Ng1.");
  const m = brief({ move: mv({ bestSan: "Ng5" }), after: { fen: START, bestSan: "Nf3" } }); // best == played: no advice
  assert.equal(m.text, "Ошибка: позиция заметно ухудшилась.");
  assert.equal(brief({ move: mv({ class: "miss" }), after: { fen: START } }).text, "Упущена возможность: сильнее было Ng1.");
});

test("good moves are praised shortly, and the engine's reply is no threat", () => {
  assert.match(brief({ move: mv({ class: "brilliant" }) }).text, /Блестящ/);
  assert.equal(brief({ move: mv({ class: "great" }) }).text, "Единственный сильный ход: остальные заметно хуже.");
  assert.equal(brief({ move: mv({ class: "best", san: "Qh5#" }) }).text, "Мат! Партия окончена.");
  assert.equal(brief({ move: mv({ class: "best", san: "Nf3", forced: true }) }).text, "Единственный ход.");
  assert.equal(brief({ move: mv({ class: "best", san: "Qxg5" }), before: { fen: "4k3/8/8/6n1/8/8/8/4K2Q w - - 0 1" } }).text, "Лучший ход: забирает коня.");
  // the Russian name is preferred; a report without it (or an unknown name) shows the English one
  assert.equal(brief({ move: mv({ class: "book", opening: { name: "Italian Game", nameRu: "Итальянская партия" } }) }).text, "Теория дебюта: Итальянская партия.");
  assert.equal(brief({ move: mv({ class: "book", opening: { name: "Italian Game" } }) }).text, "Теория дебюта: Italian Game.");
  assert.equal(brief({ move: mv({ class: "book" }) }).text, "Ход из дебютной теории.");
  assert.equal(brief({ move: mv({ class: "good", bestSan: "Nf3", san: "d3" }) }).text, "Хороший ход, но было чуть лучше: Nf3.");
  assert.equal(brief({ move: mv({ class: "good", bestSan: undefined }) }).text, "Хороший ход.");
  for (const cls of ["best", "excellent", "good", "brilliant", "great", "book"]) {
    assert.equal(brief({ move: mv({ class: cls }), after: afterTakingKnight }).threat, false, cls);
    assert.ok(brief({ move: mv({ class: cls }) }).text.length > 0, cls);
  }
  // several wordings, but one move always gets the same
  const texts = new Set([13, 14, 15, 16].map((ply) => brief({ move: mv({ class: "best", san: "Nf3", ply }) }).text));
  assert.ok(texts.size > 1);
  assert.equal(brief({ move: mv({ class: "best", san: "Nf3", ply: 13 }) }).text, brief({ move: mv({ class: "best", san: "Nf3", ply: 13 }) }).text);
});

test("a game that ended with the move gives no reply to punish", () => {
  const b = brief({ move: mv({ class: "blunder" }), after: { terminal: "stalemate", bestSan: "Kd1" } });
  assert.equal(b.threat, false);
  assert.doesNotMatch(b.text, /Kd1/);
});

test("the start of the game is a greeting", () => {
  assert.equal(brief({ move: null }).text, GREETING);
  assert.equal(brief({ move: null }).threat, false);
  assert.ok(isCostly("blunder") && isCostly("miss") && !isCostly("best") && !isCostly("book"));
});

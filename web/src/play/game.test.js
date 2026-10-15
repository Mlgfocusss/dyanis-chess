import test from "node:test";
import assert from "node:assert/strict";
import { START_FEN, buildPgn, describeFinished, describeResult, sanMoves } from "./game.js";

const base = { resigned: null, playerSide: "white" };

test("ongoing game has no result", () => {
  assert.equal(describeResult({ ...base, status: "ongoing", sideToMove: "white" }), null);
});

test("checkmate: the side to move is the one mated", () => {
  const win = describeResult({ ...base, status: "checkmate", sideToMove: "black" });
  assert.deepEqual([win.result, win.outcome, win.title, win.reason], ["1-0", "win", "Вы победили!", "Мат"]);
  assert.equal(win.subtitle, "Белые победили матом");
  const loss = describeResult({ ...base, status: "checkmate", sideToMove: "white" });
  assert.deepEqual([loss.result, loss.outcome, loss.title], ["0-1", "loss", "Вы проиграли"]);
  const asBlack = describeResult({ status: "checkmate", sideToMove: "white", resigned: null, playerSide: "black" });
  assert.deepEqual([asBlack.result, asBlack.outcome], ["0-1", "win"]);
});

test("resignation loses for the one who resigned, whatever the engine says", () => {
  const r = describeResult({ status: "ongoing", sideToMove: "white", resigned: "white", playerSide: "white" });
  assert.deepEqual([r.result, r.outcome, r.reason], ["0-1", "loss", "Сдача"]);
  assert.equal(r.subtitle, "Чёрные победили по сдаче соперника");
});

test("draws", () => {
  for (const [status, reason] of [
    ["stalemate", "Пат"],
    ["draw-fifty-move", "Правило 50 ходов"],
    ["draw-repetition", "Троекратное повторение"],
  ]) {
    const d = describeResult({ ...base, status, sideToMove: "black" });
    assert.deepEqual([d.result, d.outcome, d.title, d.reason], ["1/2-1/2", "draw", "Ничья", reason]);
  }
});

test("sanMoves drops move numbers", () => {
  assert.deepEqual(sanMoves("1. e4 e5 2. Nf3 Nc6 3. O-O"), ["e4", "e5", "Nf3", "Nc6", "O-O"]);
  assert.deepEqual(sanMoves(""), []);
  assert.deepEqual(sanMoves(undefined), []);
});

const date = new Date(2026, 9, 15);

test("PGN of a normal game", () => {
  const pgn = buildPgn({ sans: ["f3", "e5", "g4", "Qh4#"], startFen: null, result: "0-1", white: "Анна", black: "Бот", date });
  assert.equal(
    pgn,
    `[Event "Игра с ботом"]\n[Site "Dyanis Chess"]\n[Date "2026.10.15"]\n[White "Анна"]\n[Black "Бот"]\n[Result "0-1"]\n\n1. f3 e5 2. g4 Qh4# 0-1\n`
  );
  assert.ok(!pgn.includes("FEN"));
});

test("PGN from a custom position, Black to move, numbers continue from the FEN", () => {
  const fen = "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 7";
  const pgn = buildPgn({ sans: ["e5", "Nf3", "Nc6"], startFen: fen, result: "*", white: "A", black: "B", date });
  assert.ok(pgn.includes(`[SetUp "1"]\n[FEN "${fen}"]`));
  assert.ok(pgn.endsWith("7... e5 8. Nf3 Nc6 *\n"));
});

test("the standard start position needs no FEN tag, and quotes in names are replaced", () => {
  const pgn = buildPgn({ sans: ["e4"], startFen: START_FEN, result: "*", white: 'A"B\nC', black: "B", date });
  assert.ok(!pgn.includes("[FEN"));
  assert.ok(pgn.includes(`[White "A'B C"]`));
});

test("zero moves still gives a valid movetext", () => {
  assert.ok(buildPgn({ sans: [], startFen: null, result: "0-1", white: "A", black: "B", date }).endsWith("\n\n0-1\n"));
});

test("describeFinished: an online game from each point of view", () => {
  const w = describeFinished({ result: "1-0", termination: "timeout", you: "white" });
  assert.deepEqual([w.outcome, w.title, w.reason, w.subtitle], ["win", "Вы победили!", "Время вышло", "Белые победили по времени"]);
  const l = describeFinished({ result: "1-0", termination: "checkmate", you: "black" });
  assert.deepEqual([l.outcome, l.title, l.subtitle], ["loss", "Вы проиграли", "Белые победили матом"]);
  const s = describeFinished({ result: "0-1", termination: "resign", you: "" });
  assert.deepEqual([s.outcome, s.title, s.reason], ["neutral", "Чёрные победили", "Сдача"]);
  const d = describeFinished({ result: "1/2-1/2", termination: "agreement", you: "white" });
  assert.deepEqual([d.outcome, d.title, d.subtitle], ["draw", "Ничья", "Ничья: по соглашению"]);
  assert.equal(describeFinished({ result: "1/2-1/2", termination: "repetition", you: "" }).outcome, "neutral");
});

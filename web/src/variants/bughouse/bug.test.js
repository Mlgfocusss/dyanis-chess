import test from "node:test";
import assert from "node:assert/strict";
import { boardOrder, bottomSides, bugClocks, chatForRoom, describeBugEnd, movesOfBoard, partnerSeat, rematchLabel, seatBoard, seatLabel, seatOf, seatSide, teamNames, teamOf } from "./bug.js";

test("seats, partners and teams", () => {
  assert.deepEqual([0, 1, 2, 3].map(seatBoard), [0, 0, 1, 1]);
  assert.deepEqual([0, 1, 2, 3].map(seatSide), ["white", "black", "white", "black"]);
  assert.deepEqual([0, 1, 2, 3].map(partnerSeat), [3, 2, 1, 0]);
  assert.deepEqual([0, 1, 2, 3].map(teamOf), [0, 1, 1, 0]);
  for (let s = 0; s < 4; s++) {
    assert.equal(seatOf(seatBoard(s), seatSide(s)), s);
    assert.equal(teamOf(partnerSeat(s)), teamOf(s));
  }
  assert.equal(seatLabel(3), "Доска 2, чёрные");
});

test("the viewer's board comes first and partners stand with opposite colours", () => {
  assert.deepEqual(boardOrder(0), [0, 1]);
  assert.deepEqual(boardOrder(3), [1, 0]);
  assert.deepEqual(boardOrder(-1), [0, 1]);
  assert.deepEqual(bottomSides(1), ["black", "white"]); // I am Black on board 1, my partner is White on board 2
  assert.deepEqual(bottomSides(2), ["black", "white"]); // I am White on board 2: board 1 shows my partner (Black) at the bottom
  assert.deepEqual(bottomSides(2, true), ["white", "white"]);
  assert.deepEqual(bottomSides(-1), ["white", "white"]);
});

test("clocks count down only the seats that run", () => {
  const clock = { ms: [60000, 50000, 40000, 30000], running: [0, 3], serverNow: 1 };
  assert.deepEqual(bugClocks(clock, 1000, 3500), [57500, 50000, 40000, 27500]);
  assert.deepEqual(bugClocks(clock, 1000, 500), [60000, 50000, 40000, 30000]);
  assert.deepEqual(bugClocks({ ...clock, running: [] }, 0, 9999), [60000, 50000, 40000, 30000]);
  assert.deepEqual(bugClocks({ ...clock, ms: [1000, 0, 0, 0], running: [0] }, 0, 5000), [0, 0, 0, 0]);
  assert.equal(bugClocks(undefined, 0, 0), null);
});

const players = [{ name: "Анна" }, { name: "Карл" }, { name: "Давид" }, { name: "Борис" }];

test("the end is told from the viewer's team", () => {
  const mate = { result: "1-0", termination: "checkmate", players };
  assert.equal(teamNames(players, 0), "Анна и Борис");
  assert.equal(teamNames(players, 1), "Карл и Давид");
  const win = describeBugEnd({ ...mate, yourSeat: 3 });
  assert.equal(win.outcome, "win");
  assert.equal(win.title, "Ваша команда победила!");
  assert.match(win.subtitle, /Анна и Борис/);
  assert.match(win.subtitle, /матом/);
  assert.equal(describeBugEnd({ ...mate, yourSeat: 1 }).outcome, "loss");
  const watcher = describeBugEnd({ ...mate, yourSeat: -1 });
  assert.equal(watcher.outcome, "neutral");
  assert.equal(watcher.title, "Победили Анна и Борис");
  assert.equal(describeBugEnd({ result: "0-1", termination: "timeout", yourSeat: 2, players }).outcome, "win");
  assert.equal(describeBugEnd({ result: "0-1", termination: "resign", yourSeat: 2, players }).reason, "Сдача");
});

test("the chat of four goes through the shared component", () => {
  const { game, you } = chatForRoom({ chat: [{ seat: 2, text: "привет", at: 5 }], players }, 2);
  assert.deepEqual(game.chat, [{ side: "s2", text: "привет", at: 5 }]);
  assert.equal(game.s2.name, "Давид");
  assert.equal(you, "s2");
  assert.equal(chatForRoom({ players }, -1).you, "");
});

test("rematch wording and the moves of a board", () => {
  assert.equal(rematchLabel(undefined, 0), "Реванш");
  assert.equal(rematchLabel({ offers: [false, true, false, false] }, 0), "Принять реванш (1/4)");
  assert.equal(rematchLabel({ offers: [true, true, false, false] }, 0), "Ждём остальных (2/4)");
  const moves = [{ board: 0, san: "e4" }, { board: 1, san: "d4" }, { board: 0, san: "e5" }];
  assert.deepEqual(movesOfBoard(moves, 0).map((m) => m.san), ["e4", "e5"]);
});

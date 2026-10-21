import { test } from "node:test";
import assert from "node:assert/strict";
import {
  accuracyChart,
  accuracyNote,
  activeDays,
  activityCells,
  dayLabel,
  daysText,
  historyRows,
  isEmpty,
  percent,
  percentText,
  plural,
  recordBar,
  recordText,
  rushRows,
  signedDelta,
  sideText,
  streakBadge,
  weakRows,
  whenText,
} from "./stats.js";

test("Russian plurals", () => {
  assert.equal(daysText(1), "1 день");
  assert.equal(daysText(2), "2 дня");
  assert.equal(daysText(5), "5 дней");
  assert.equal(daysText(11), "11 дней");
  assert.equal(daysText(21), "21 день");
  assert.equal(daysText(112), "112 дней");
  assert.equal(plural(0, "задача", "задачи", "задач"), "задач");
});

test("the streak badge shows only a streak that is on", () => {
  assert.equal(streakBadge({ current: 4 }), "Серия: 4 дн.");
  assert.equal(streakBadge({ current: 0 }), "");
  assert.equal(streakBadge(null), "");
});

test("percentages and the result bar", () => {
  assert.equal(percent(5, 8), 63);
  assert.equal(percent(0, 0), null);
  assert.equal(percentText(0, 0), "—");
  assert.equal(percentText(1, 3), "33%");
  assert.deepEqual(recordBar({ total: 0 }), { win: 0, draw: 0, loss: 0 });
  const bar = recordBar({ total: 3, wins: 1, draws: 1, losses: 1 });
  assert.equal(bar.win + bar.draw + bar.loss, 100);
  assert.deepEqual(recordBar({ total: 4, wins: 4, draws: 0, losses: 0 }), { win: 100, draw: 0, loss: 0 });
  assert.equal(recordText({ wins: 3, draws: 1, losses: 2 }), "3 / 1 / 2");
  assert.equal(recordText(undefined), "0 / 0 / 0");
});

test("signed rating changes", () => {
  assert.equal(signedDelta(12), "+12");
  assert.equal(signedDelta(-16), "−16");
  assert.equal(signedDelta(0), "±0");
});

test("the accuracy note says honestly what was averaged", () => {
  assert.equal(accuracyNote({ sample: 0, known: 0 }), "");
  assert.match(accuracyNote({ sample: 5, known: 5 }), /за ваш цвет/);
  assert.match(accuracyNote({ sample: 5, known: 0 }), /среднее по обеим сторонам/);
  assert.match(accuracyNote({ sample: 5, known: 2 }), /2 из 5/);
  assert.equal(sideText("white"), "за белых");
  assert.equal(sideText("both"), "среднее по обеим сторонам");
  assert.equal(sideText("?"), "среднее по обеим сторонам");
});

test("accuracy chart geometry", () => {
  assert.deepEqual(accuracyChart([], null, 400, 200).points, []);
  assert.deepEqual(accuracyChart([{ accuracy: 80 }], 80, 10, 200).points, []); // too narrow to draw

  const series = [{ accuracy: 60 }, { accuracy: 90 }, { accuracy: 75 }];
  const c = accuracyChart(series, 75, 400, 200);
  const { left, right, top, bottom } = c.pad;
  assert.equal(c.points.length, 3);
  assert.equal(c.points[0].x, left);
  assert.equal(c.points[2].x, 400 - right);
  // the best result is the highest point, the worst the lowest, all inside the plot
  assert.ok(c.points[1].y < c.points[2].y && c.points[2].y < c.points[0].y);
  for (const p of c.points) assert.ok(p.y >= top && p.y <= 200 - bottom);
  assert.ok(c.line.startsWith("M") && c.line.split("L").length === 3);
  // the scale covers the data in steps of ten and the mean line sits between the extremes
  assert.ok(c.ticks[0].value <= 55 && c.ticks.at(-1).value >= 95);
  assert.ok(c.ticks.every((t, i) => i === 0 || t.value - c.ticks[i - 1].value === 10));
  assert.ok(c.avgY > c.points[1].y && c.avgY < c.points[0].y);

  // one point sits in the middle; a flat series still gets a scale at least 30 tall
  const one = accuracyChart([{ accuracy: 100 }], 100, 400, 200);
  assert.equal(one.points[0].x, left + (400 - left - right) / 2);
  assert.ok(one.ticks.at(-1).value - one.ticks[0].value >= 30 && one.ticks.at(-1).value <= 100);
  const flat = accuracyChart([{ accuracy: 50 }, { accuracy: 50 }], 50, 400, 200);
  assert.ok(flat.ticks.at(-1).value - flat.ticks[0].value >= 30);
});

test("activity cells run from the first day with levels by the busiest day", () => {
  assert.deepEqual(activityCells(null), []);
  const days = new Array(30).fill(0);
  days[0] = 1;
  days[28] = 4;
  days[29] = 2;
  const cells = activityCells({ from: "2026-09-21", days });
  assert.equal(cells.length, 30);
  assert.equal(cells[0].date, "2026-09-21");
  assert.equal(cells[29].date, "2026-10-20");
  assert.deepEqual(cells.map((c) => c.level).filter((l) => l > 0), [1, 4, 2]);
  assert.equal(cells[1].level, 0);
  assert.equal(cells[29].label, "20 окт: 2 задачи");
  assert.equal(cells[1].label, "22 сен: без задач");
  assert.equal(activeDays({ days }), 3);
  // a window across a month boundary
  assert.equal(activityCells({ from: "2026-02-27", days: [0, 0, 0, 0] })[3].date, "2026-03-02");
  assert.equal(dayLabel("2026-01-05"), "5 янв");
});

test("weak themes get Russian names, unknown ones keep their id", () => {
  const rows = weakRows([
    { theme: "fork", attempts: 4, failed: 3 },
    { theme: "someNewTheme", attempts: 2, failed: 1 },
  ]);
  assert.equal(rows[0].name, "Вилка");
  assert.equal(rows[0].share, 75);
  assert.equal(rows[1].name, "someNewTheme");
  assert.deepEqual(weakRows(undefined), []);
});

test("times of the history are relative to now", () => {
  const now = new Date(2026, 9, 20, 15, 0);
  assert.equal(whenText(new Date(2026, 9, 20, 9, 5).toISOString(), now), "сегодня, 09:05");
  assert.equal(whenText(new Date(2026, 9, 19, 23, 59).toISOString(), now), "вчера, 23:59");
  assert.equal(whenText(new Date(2026, 8, 3, 7, 0).toISOString(), now), "3 сен, 07:00");
  assert.equal(whenText("nonsense", now), "");
});

test("history rows link to the puzzle", () => {
  const now = new Date(2026, 9, 20, 15, 0);
  const rows = historyRows(
    [{ puzzle: "00sHx", solved: false, before: 1250, after: 1234, delta: -16, at: new Date(2026, 9, 20, 14, 0).toISOString() }],
    now,
  );
  assert.deepEqual(rows, [
    { id: "00sHx", href: "#puzzles/00sHx", solved: false, rating: 1234, delta: "−16", when: "сегодня, 14:00" },
  ]);
  assert.deepEqual(historyRows(undefined), []);
});

test("storm rows keep a fixed order and skip modes never played", () => {
  assert.deepEqual(rushRows({ "5min": { best: 20, runs: 3 }, "3min": { best: 14, runs: 2 } }).map((r) => r.mode), ["3min", "5min"]);
  assert.deepEqual(rushRows({}), []);
  assert.deepEqual(rushRows(null), []);
});

test("an empty profile is recognised", () => {
  const blank = { games: { total: 0 }, reviews: { count: 0 }, puzzles: { attempts: 0 }, streak: { best: 0 } };
  assert.equal(isEmpty(blank), true);
  assert.equal(isEmpty(null), true);
  assert.equal(isEmpty({ ...blank, games: { total: 2 } }), false);
});

import test from "node:test";
import assert from "node:assert/strict";
import {
  PUZZLE_THEME_KEY,
  accuracyLabel,
  classWord,
  costlyCaption,
  dayPercent,
  dayText,
  dynamicsChart,
  itemPercent,
  kindIcon,
  moveLabel,
  needText,
  num,
  phaseBars,
  progressText,
  rateTiles,
  rememberTheme,
  skippedNote,
  trendTone,
} from "./trainer.js";

test("itemPercent and progressText follow the goal, and a point without a goal has no bar", () => {
  assert.equal(itemPercent({ progress: 3, goal: 5 }), 60);
  assert.equal(itemPercent({ progress: 9, goal: 5 }), 100);
  assert.equal(itemPercent({ progress: 0, goal: 5 }), 0);
  assert.equal(itemPercent({ progress: 0, goal: 0 }), null);
  assert.equal(itemPercent(null), null);
  assert.equal(progressText({ progress: 3, goal: 5 }), "3 из 5");
  assert.equal(progressText({ progress: 0, goal: 0 }), "");
});

test("dayText and dayPercent", () => {
  assert.equal(dayText({ done: 2, total: 3 }), "Выполнено 2 из 3");
  assert.equal(dayText({ done: 3, total: 3 }), "Выполнено всё: 3 из 3");
  assert.equal(dayText({ done: 0, total: 0 }), "");
  assert.equal(dayPercent({ done: 1, total: 3 }), 33);
  assert.equal(dayPercent({ done: 0, total: 0 }), 0);
});

test("kindIcon falls back to a star", () => {
  assert.equal(kindIcon("puzzles"), "puzzles");
  assert.equal(kindIcon("repertoire"), "openings");
  assert.equal(kindIcon("lesson"), "lessons");
  assert.equal(kindIcon("play"), "bots");
  assert.equal(kindIcon("new thing"), "star");
});

test("num writes a decimal comma and drops a trailing zero", () => {
  assert.equal(num(61), "61");
  assert.equal(num(88.4), "88,4");
  assert.equal(num(88.46), "88,5");
  assert.equal(num(null), "—");
  assert.equal(num(undefined), "—");
});

const phases = (a, b, c) => [
  { id: "opening", name: "Дебют", accuracy: a, moves: 40 },
  { id: "middlegame", name: "Миттельшпиль", accuracy: b, moves: 60 },
  { id: "endgame", name: "Эндшпиль", accuracy: c, moves: 20 },
];

test("phaseBars marks the best and the weak stage when the gap is wide", () => {
  const bars = phaseBars(phases(88, 84, 61));
  assert.deepEqual(
    bars.map((b) => b.kind),
    ["best", "mid", "weak"],
  );
  assert.equal(bars[0].height, 88);
  assert.equal(bars[2].name, "Эндшпиль");
});

test("phaseBars paints nothing red when the stages are close, and handles missing numbers", () => {
  assert.deepEqual(
    phaseBars(phases(80, 78, 75)).map((b) => b.kind),
    ["mid", "mid", "mid"],
  );
  const some = phaseBars(phases(null, 84, 61));
  assert.deepEqual(
    some.map((b) => b.kind),
    ["none", "best", "weak"],
  );
  assert.equal(some[0].height, 0);
  // one stage alone has nothing to be compared with
  assert.deepEqual(
    phaseBars(phases(null, null, 61)).map((b) => b.kind),
    ["none", "none", "mid"],
  );
  assert.deepEqual(phaseBars(undefined), []);
  // a very low column is still seen
  assert.equal(phaseBars(phases(1, 90, 90))[0].height, 4);
  assert.equal(accuracyLabel(null), "мало ходов");
  assert.equal(accuracyLabel(61), "61%");
});

test("rateTiles list the three kinds of slips", () => {
  const tiles = rateTiles({ inaccuracies: 6.1, mistakes: 3, blunders: 1.5 });
  assert.deepEqual(
    tiles.map((t) => [t.id, t.value]),
    [
      ["inaccuracies", "6,1"],
      ["mistakes", "3"],
      ["blunders", "1,5"],
    ],
  );
  assert.equal(rateTiles(undefined)[0].value, "0");
});

test("trendTone", () => {
  assert.equal(trendTone({ delta: 3.2 }), "up");
  assert.equal(trendTone({ delta: 1 }), "up");
  assert.equal(trendTone({ delta: 0.5 }), "flat");
  assert.equal(trendTone({ delta: -1 }), "down");
  assert.equal(trendTone(null), "flat");
});

test("moveLabel, classWord and costlyCaption", () => {
  assert.equal(moveLabel({ number: 7, black: false, san: "Ng5" }), "7.Ng5");
  assert.equal(moveLabel({ number: 7, black: true, san: "Nf6" }), "7...Nf6");
  assert.equal(classWord("blunder"), "Зевок");
  assert.equal(classWord("mistake"), "Ошибка");
  assert.equal(classWord("inaccuracy"), "Неточность");
  assert.equal(classWord(""), "Ошибка");
  const cap = costlyCaption({ opponent: "Лиза", at: "2026-10-18T12:00:00Z" });
  assert.match(cap, /^соперник Лиза, \d{1,2} [а-я]+$/);
  assert.equal(costlyCaption({ at: "nonsense" }), "");
});

test("dynamicsChart puts the points in the box and keeps a 30 point window", () => {
  const series = [
    { id: "a", accuracy: 60 },
    { id: "b", accuracy: 75 },
    { id: "c", accuracy: 90 },
  ];
  const c = dynamicsChart(series, 75, 400, 200);
  assert.equal(c.points.length, 3);
  assert.ok(c.points[0].x < c.points[1].x && c.points[1].x < c.points[2].x);
  assert.ok(c.points[0].y > c.points[2].y, "a higher accuracy is higher on the page");
  assert.ok(c.points.every((p) => p.x >= c.pad.left && p.x <= 400 - c.pad.right && p.y >= c.pad.top && p.y <= 200 - c.pad.bottom));
  assert.ok(c.ticks.at(-1).value - c.ticks[0].value >= 30);
  assert.ok(c.avgY > c.points[2].y && c.avgY < c.points[0].y);
  assert.match(c.line, /^M[\d.]+ [\d.]+ L/);
  // one review sits in the middle; nothing to draw without data or width
  assert.equal(dynamicsChart([{ id: "a", accuracy: 80 }], null, 400, 200).points[0].x, (34 + (400 - 34 - 14) / 2));
  assert.equal(dynamicsChart([{ id: "a", accuracy: 80 }], null, 400, 200).avgY, null);
  assert.equal(dynamicsChart([], 0, 400, 200).points.length, 0);
  assert.equal(dynamicsChart(series, 75, 30, 200).points.length, 0);
  // the window never leaves 0..100
  const edge = dynamicsChart([{ id: "a", accuracy: 99 }, { id: "b", accuracy: 100 }], 99.5, 400, 200);
  assert.ok(edge.ticks.at(-1).value <= 100 && edge.ticks[0].value >= 0);
});

test("skippedNote and needText", () => {
  assert.equal(skippedNote({ skipped: 0 }), "");
  assert.match(skippedNote({ skipped: 1 }), /^1 разбор я пропустил/);
  assert.match(skippedNote({ skipped: 3 }), /^3 разбора я пропустил/);
  assert.match(skippedNote({ skipped: 5 }), /^5 разборов я пропустил/);
  assert.match(skippedNote({ skipped: 11 }), /^11 разборов/);
  assert.equal(needText({ used: 1, need: 3 }), "Разобрано с вашим цветом: 1 из 3");
  assert.equal(needText({ used: 7, need: 3 }), "Разобрано с вашим цветом: 3 из 3");
  assert.equal(needText({}), "Разобрано с вашим цветом: 0 из 3");
});

test("rememberTheme stores the theme under the key of the puzzle page and ignores a broken storage", () => {
  const data = {};
  const storage = { setItem: (k, v) => (data[k] = v) };
  rememberTheme("fork", storage);
  assert.equal(data[PUZZLE_THEME_KEY], "fork");
  assert.equal(PUZZLE_THEME_KEY, "dyanis.puzzleTheme");
  rememberTheme("", storage);
  rememberTheme(undefined, storage);
  assert.deepEqual(Object.keys(data), [PUZZLE_THEME_KEY]);
  assert.doesNotThrow(() =>
    rememberTheme("pin", {
      setItem() {
        throw new Error("blocked");
      },
    }),
  );
  assert.doesNotThrow(() => rememberTheme("pin", null));
});

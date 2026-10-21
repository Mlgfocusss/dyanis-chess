import test from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULT_MISS,
  boardArrows,
  continueLesson,
  courseProgress,
  highlightCss,
  hintSquare,
  judge,
  legalMoves,
  lessonPercent,
  lessonStatus,
  mainLine,
  minutesLabel,
  mistakesLabel,
  parseHash,
  parseHighlight,
  progressBody,
  resultSummary,
  ruPlural,
  sectionProgress,
  solutionLength,
  squaresOf,
  startStep,
  stepCounter,
  stepsLabel,
  wrongMessage,
} from "./lesson.js";

// a two-move exercise: e2e4 e7e5 then g1f3 or d2d4 ends it; also a single mate-like branch
const tree = {
  fen: "start",
  legal: ["e2e4", "d2d4", "g1f3"],
  moves: [
    {
      uci: "e2e4",
      san: "e4",
      fen: "f1",
      reply: { uci: "e7e5", san: "e5", fen: "f2" },
      next: {
        fen: "f2",
        legal: ["g1f3", "d2d4"],
        moves: [
          { uci: "g1f3", san: "Nf3", fen: "f3" },
          { uci: "d2d4", san: "d4", fen: "f4" },
        ],
      },
    },
    { uci: "d2d4", san: "d4", fen: "f5" },
  ],
};

test("judge: the right move of a branch continues, a single move ends the exercise", () => {
  assert.deepEqual(judge(tree, "a2a3"), { kind: "wrong" });
  const j = judge(tree, "e2e4");
  assert.equal(j.kind, "right");
  assert.equal(j.done, false);
  assert.equal(j.accept.reply.uci, "e7e5");
  const end = judge(tree, "d2d4");
  assert.equal(end.done, true);
  assert.equal(judge(j.accept.next, "g1f3").done, true);
  assert.deepEqual(judge(null, "e2e4"), { kind: "wrong" });
});

test("legalMoves has the shape of the board", () => {
  assert.deepEqual(legalMoves(tree), [{ uci: "e2e4" }, { uci: "d2d4" }, { uci: "g1f3" }]);
  assert.deepEqual(legalMoves(null), []);
});

test("wrongMessage: an exact move, then a piece, the step's own text, the default", () => {
  const step = {
    miss: "Нет.",
    mistakes: [
      { move: "a2", message: "Пешка a." },
      { move: "a2a3", message: "Ровно a3." },
      { move: "h2h4", message: "Только в начале.", at: 0 },
      { move: "g1h3", message: "Позже.", at: 1 },
    ],
  };
  assert.equal(wrongMessage(step, "a2a3", 0), "Ровно a3.");
  assert.equal(wrongMessage(step, "a2a4", 0), "Пешка a.");
  assert.equal(wrongMessage(step, "h2h4", 0), "Только в начале.");
  assert.equal(wrongMessage(step, "h2h4", 1), "Нет.", "a mistake of another move does not apply");
  assert.equal(wrongMessage(step, "g1h3", 1), "Позже.");
  assert.equal(wrongMessage(step, "b1a3", 0), "Нет.");
  assert.equal(wrongMessage(step, "b1a3", 0, 2), "Нет. Можно взять подсказку.");
  assert.equal(wrongMessage({}, "b1a3", 0), DEFAULT_MISS);
  // "at": 0 is a real position, not "any"
  assert.equal(wrongMessage({ mistakes: [{ move: "a2a3", message: "x", at: 0 }] }, "a2a3", 1), DEFAULT_MISS);
});

test("hintSquare, mainLine and solutionLength follow the first solution", () => {
  assert.equal(hintSquare(tree), "e2");
  assert.equal(hintSquare({ moves: [] }), null);
  const line = mainLine(tree);
  assert.deepEqual(
    line.map((m) => [m.uci, m.mine]),
    [
      ["e2e4", true],
      ["e7e5", false],
      ["g1f3", true],
    ]
  );
  assert.equal(solutionLength(tree), 2);
  assert.deepEqual(mainLine(null), []);
});

test("parseHighlight and highlightCss use tokens and refuse bad squares", () => {
  assert.deepEqual(parseHighlight("e4"), { square: "e4", token: "primary" });
  assert.deepEqual(parseHighlight("e4:danger"), { square: "e4", token: "danger" });
  assert.equal(parseHighlight("i9"), null);
  assert.equal(parseHighlight("e4:pink"), null);
  assert.equal(parseHighlight(7), null);
  const css = highlightCss(["e4:warn", "zz", "a1"], ".b");
  assert.match(css, /\.b \[data-square="e4"\]::before/);
  assert.match(css, /rgba\(var\(--warn-rgb\)/);
  assert.match(css, /\[data-square="a1"\]/);
  assert.doesNotMatch(css, /zz/);
  assert.doesNotMatch(css, /#[0-9a-f]{3,6}\b/i, "no hard-coded colour");
  assert.equal(highlightCss(undefined, ".b"), "");
});

test("boardArrows turns tokens into colours and drops bad squares", () => {
  const out = boardArrows([["e2", "e4", "danger"], ["a1", "a9", "info"], ["b1", "c3", "nonsense"], ["g1", "f3"]], (t) => `c-${t}`);
  assert.deepEqual(out, [
    ["e2", "e4", "c-danger"],
    ["b1", "c3", "c-info"],
    ["g1", "f3", "c-info"],
  ]);
  assert.deepEqual(boardArrows(undefined, (t) => t), []);
});

test("squaresOf ignores a pass", () => {
  assert.deepEqual(squaresOf("e2e4"), { from: "e2", to: "e4" });
  assert.deepEqual(squaresOf("e7e8q"), { from: "e7", to: "e8" });
  assert.equal(squaresOf("--"), null);
  assert.equal(squaresOf(""), null);
});

test("lessonStatus, startStep and lessonPercent", () => {
  assert.equal(lessonStatus(undefined), "new");
  assert.equal(lessonStatus({ step: 0, completed: false }), "new");
  assert.equal(lessonStatus({ step: 3, completed: false }), "started");
  assert.equal(lessonStatus({ step: 6, completed: true }), "done");

  assert.equal(startStep(undefined, 6), 0);
  assert.equal(startStep({ step: 3, completed: false }, 6), 3);
  assert.equal(startStep({ step: 6, completed: false }, 6), 5, "never past the last step");
  assert.equal(startStep({ step: 6, completed: true }, 6), 0, "a finished lesson is played again");

  assert.equal(lessonPercent(undefined, 6), 0);
  assert.equal(lessonPercent({ step: 3, completed: false }, 6), 50);
  assert.equal(lessonPercent({ step: 2, completed: true }, 6), 100);
  assert.equal(lessonPercent({ step: 1, completed: false }, 0), 0);
});

const catalog = {
  total: 4,
  completed: 1,
  next: "b",
  sections: [
    {
      id: "s1",
      lessons: [
        { id: "a", progress: { step: 5, completed: true } },
        { id: "b" },
      ],
    },
    { id: "s2", lessons: [{ id: "c", progress: { step: 2, completed: false } }, { id: "d" }] },
  ],
};

test("courseProgress and sectionProgress", () => {
  assert.deepEqual(courseProgress(catalog), { done: 1, total: 4, percent: 25 });
  assert.deepEqual(courseProgress(undefined), { done: 0, total: 0, percent: 0 });
  assert.deepEqual(sectionProgress(catalog.sections[0]), { done: 1, total: 2 });
  assert.deepEqual(sectionProgress(undefined), { done: 0, total: 0 });
});

test("continueLesson: the started lesson first, then the server's next", () => {
  assert.equal(continueLesson(catalog).id, "c");
  const noStarted = { ...catalog, sections: [{ id: "s1", lessons: [{ id: "a", progress: { step: 5, completed: true } }, { id: "b" }] }] };
  assert.equal(continueLesson(noStarted).id, "b");
  assert.equal(continueLesson({ sections: [], next: "" }), null);
  assert.equal(continueLesson(undefined), null);
});

test("progressBody clamps and marks the end", () => {
  assert.deepEqual(progressBody(2, 6, 1), { step: 2, mistakes: 1, done: false });
  assert.deepEqual(progressBody(6, 6, 0), { step: 6, mistakes: 0, done: true });
  assert.deepEqual(progressBody(9, 6, -2), { step: 6, mistakes: 0, done: true });
  assert.deepEqual(progressBody(-1, 6, 0), { step: 0, mistakes: 0, done: false });
});

test("parseHash", () => {
  assert.deepEqual(parseHash("#lessons"), { view: "catalog", id: "" });
  assert.deepEqual(parseHash("#lessons/rook"), { view: "lesson", id: "rook" });
  assert.deepEqual(parseHash("#lessons/en-passant"), { view: "lesson", id: "en-passant" });
  assert.deepEqual(parseHash("#lessons/../x"), { view: "catalog", id: "" });
  assert.deepEqual(parseHash("#puzzles/rook"), { view: "catalog", id: "" });
  assert.deepEqual(parseHash(""), { view: "catalog", id: "" });
});

test("Russian numerals", () => {
  assert.equal(ruPlural(1, "а", "б", "в"), "а");
  assert.equal(ruPlural(2, "а", "б", "в"), "б");
  assert.equal(ruPlural(5, "а", "б", "в"), "в");
  assert.equal(ruPlural(11, "а", "б", "в"), "в");
  assert.equal(ruPlural(21, "а", "б", "в"), "а");
  assert.equal(ruPlural(24, "а", "б", "в"), "б");
  assert.equal(minutesLabel(5), "5 минут");
  assert.equal(minutesLabel(1), "1 минута");
  assert.equal(minutesLabel(2), "2 минуты");
  assert.equal(stepsLabel(6), "6 шагов");
  assert.equal(stepsLabel(7), "7 шагов");
  assert.equal(mistakesLabel(0), "0 ошибок");
  assert.equal(mistakesLabel(1), "1 ошибка");
  assert.equal(mistakesLabel(3), "3 ошибки");
  assert.equal(stepCounter(2, 7), "Шаг 3 из 7");
});

test("resultSummary", () => {
  assert.match(resultSummary(0).title, /без ошибок/);
  assert.match(resultSummary(2).text, /2/);
  assert.match(resultSummary(9).text, /ещё раз/);
});

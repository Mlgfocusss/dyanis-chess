// Pure logic of the lessons, with no imports, so `npm test` covers it.
//
// A lesson from the server (backend/internal/lessons) is a list of steps. A text step has a position,
// arrows, highlighted squares and perhaps a demonstration: moves with the position after each. An
// exercise has a tree: { fen, legal: [uci], moves: [{ uci, san, fen, mate?, reply?: {uci, san, fen}, next?: tree }] }
// where `moves` are the right moves of the person at that position. A right move that has a `reply`
// is answered by the opponent, and then the person moves again in `next`; without a `next` the
// exercise is done after the reply; without a `reply` it is done after the move itself.

export const PASS = "--"; // a demonstration "move" that only hands the turn over (no piece moves)
export const DEFAULT_MISS = "Не то. Попробуйте ещё раз.";

// ------------------------------------------------------------------ exercises

// What the person's move `uci` means in the position `node`:
//   { kind: "wrong" }
//   { kind: "right", accept, done }   accept is the move from the tree; done: the exercise is over
//                                     after it (and after the reply, if there is one)
export function judge(node, uci) {
  const accept = (node?.moves || []).find((m) => m.uci === uci);
  if (!accept) return { kind: "wrong" };
  return { kind: "right", accept, done: !accept.next };
}

// The legal moves for the board, in the shape Board.jsx takes.
export function legalMoves(node) {
  return ((node && node.legal) || []).map((uci) => ({ uci }));
}

// What to tell about a wrong move. A mistake of the step wins if its `move` is this very move, or a
// square (the piece standing there) and its `at` is not another move of the exercise; an exact
// move is preferred to a square. `depth` is the number of the person's move in the exercise (0 first).
// `wrongHere` is how many wrong moves this step has seen, for the nudge toward the hint.
export function wrongMessage(step, uci, depth, wrongHere = 1) {
  const list = (step.mistakes || []).filter((m) => m.at === undefined || m.at === null || m.at === depth);
  const exact = list.find((m) => m.move === uci);
  const byPiece = list.find((m) => m.move.length === 2 && uci.slice(0, 2) === m.move);
  const found = exact || byPiece;
  if (found) return found.message;
  const base = step.miss || DEFAULT_MISS;
  return wrongHere >= 2 ? `${base} Можно взять подсказку.` : base;
}

// The square the hint points at: where the piece of the main solution stands.
export function hintSquare(node) {
  const first = node?.moves?.[0];
  return first ? first.uci.slice(0, 2) : null;
}

// The moves of the main solution from `node`, one after another, for "Показать решение":
// [{ uci, san, fen, mine }] with the person's moves and the opponent's replies.
export function mainLine(node) {
  const out = [];
  let n = node;
  while (n && n.moves && n.moves.length) {
    const a = n.moves[0];
    out.push({ uci: a.uci, san: a.san, fen: a.fen, mine: true });
    if (!a.reply) break;
    out.push({ uci: a.reply.uci, san: a.reply.san, fen: a.reply.fen, mine: false });
    n = a.next;
  }
  return out;
}

// How many moves of the person the exercise has along the main solution.
export function solutionLength(node) {
  return mainLine(node).filter((m) => m.mine).length;
}

// ---------------------------------------------------------------- squares and colours

const SQUARE = /^[a-h][1-8]$/;
export const TOKENS = ["primary", "info", "warn", "danger"];

// "e4" or "e4:danger" as { square, token }; null for anything else.
export function parseHighlight(h) {
  if (typeof h !== "string") return null;
  const [square, token = "primary"] = h.split(":");
  if (!SQUARE.test(square) || !TOKENS.includes(token)) return null;
  return { square, token };
}

// CSS that tints the highlighted squares of the board inside `scope` (a selector), using the
// colour tokens so that a light theme recolours them. The board draws every square as [data-square].
export function highlightCss(highlights, scope) {
  return (highlights || [])
    .map(parseHighlight)
    .filter(Boolean)
    .map(
      ({ square, token }) =>
        `${scope} [data-square="${square}"]::before { content: ""; position: absolute; inset: 0; z-index: 0; pointer-events: none; background: rgba(var(--${token}-rgb), 0.5); }`
    )
    .join("\n");
}

// Arrows [from, to, token] as the board takes them: [from, to, colour], with the token turned into
// a colour by `colourOf`.
export function boardArrows(arrows, colourOf) {
  return (arrows || [])
    .filter((a) => Array.isArray(a) && SQUARE.test(a[0]) && SQUARE.test(a[1]))
    .map((a) => [a[0], a[1], colourOf(TOKENS.includes(a[2]) ? a[2] : "info")]);
}

// Squares of a move, for the last-move highlight; null for a pass or nothing.
export function squaresOf(uci) {
  return uci && uci !== PASS && uci.length >= 4 ? { from: uci.slice(0, 2), to: uci.slice(2, 4) } : null;
}

// ------------------------------------------------------------------ progress

// "new" | "started" | "done" for a lesson of the catalog.
export function lessonStatus(progress) {
  if (!progress) return "new";
  if (progress.completed) return "done";
  return progress.step > 0 ? "started" : "new";
}

// The step a lesson is opened at: the first one for a new or finished lesson (a finished one is
// played again), otherwise where the person stopped.
export function startStep(progress, total) {
  if (!progress || progress.completed) return 0;
  return Math.max(0, Math.min(progress.step, total - 1));
}

// How full the bar of a lesson is, in per cent.
export function lessonPercent(progress, total) {
  if (!progress || !total) return 0;
  if (progress.completed) return 100;
  return Math.max(0, Math.min(100, Math.round((progress.step / total) * 100)));
}

// The whole course: { done, total, percent } over a catalog from GET /api/lessons.
export function courseProgress(catalog) {
  const total = catalog?.total || 0;
  const done = catalog?.completed || 0;
  return { done, total, percent: total ? Math.round((done / total) * 100) : 0 };
}

// { done, total } for one section of the catalog.
export function sectionProgress(section) {
  const lessons = section?.lessons || [];
  return { done: lessons.filter((l) => l.progress?.completed).length, total: lessons.length };
}

// The lesson that "Продолжить" opens: one that was started and not finished comes first, then the
// first one not yet finished (the server names it in `next`).
export function continueLesson(catalog) {
  for (const s of catalog?.sections || []) {
    for (const l of s.lessons) {
      if (lessonStatus(l.progress) === "started") return l;
    }
  }
  for (const s of catalog?.sections || []) {
    for (const l of s.lessons) {
      if (l.id === catalog.next) return l;
    }
  }
  return null;
}

// The body of POST /api/lessons/<id>/progress after `stepsDone` steps of `total`.
export function progressBody(stepsDone, total, mistakes) {
  return { step: Math.max(0, Math.min(stepsDone, total)), mistakes: Math.max(0, mistakes), done: stepsDone >= total };
}

// ------------------------------------------------------------------- the address

// "#lessons" is the catalogue, "#lessons/<id>" a lesson.
export function parseHash(hash) {
  const [page, id] = String(hash || "").replace(/^#/, "").split("/");
  if (page !== "lessons" || !id || !/^[a-z0-9-]+$/.test(id)) return { view: "catalog", id: "" };
  return { view: "lesson", id };
}

// ---------------------------------------------------------------------- words

export function ruPlural(n, one, few, many) {
  const m10 = Math.abs(n) % 10;
  const m100 = Math.abs(n) % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

export function minutesLabel(n) {
  return `${n} ${ruPlural(n, "минута", "минуты", "минут")}`;
}

export function stepsLabel(n) {
  return `${n} ${ruPlural(n, "шаг", "шага", "шагов")}`;
}

export function mistakesLabel(n) {
  return `${n} ${ruPlural(n, "ошибка", "ошибки", "ошибок")}`;
}

// The end of a lesson: a title and a line about the mistakes.
export function resultSummary(mistakes) {
  if (mistakes === 0) return { title: "Урок пройден без ошибок!", text: "Ни одной ошибки: отличная работа." };
  if (mistakes <= 3) return { title: "Урок пройден!", text: `Ошибок: ${mistakes}. Совсем немного.` };
  return { title: "Урок пройден!", text: `Ошибок: ${mistakes}. Пройдите урок ещё раз, чтобы закрепить.` };
}

// "Шаг 3 из 7".
export function stepCounter(index, total) {
  return `Шаг ${index + 1} из ${total}`;
}

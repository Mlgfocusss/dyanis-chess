import { tx, getLanguage, serverMsg, tr } from "../i18n/index.js";
// Pure helpers of the trainer page ("Тренер"): words, shares and the geometry of the charts, and the
// two calls to the server. No React, so `npm test` covers the logic. The texts of the plan and of the
// recommendations are made by the server (backend/internal/trainer); this file only lays them out.

// The puzzle page remembers the chosen theme under this key (puzzles/PuzzlesPage.jsx).
export const PUZZLE_THEME_KEY = "dyanis.puzzleTheme";

// The accuracy gap between the best and the worst stage that the page paints red (the same
// number as PhaseGap in backend/internal/trainer/rules.go).
export const PHASE_GAP = 8;

// ---------------------------------------------------------------- server

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) throw new Error(serverMsg(body?.error) || tr("common.serverReplied", { status: res.status }));
  return body;
}

// GET /api/trainer: the plan, the picture of the play and the advice. hour is the local hour of the
// browser (the server does not know the time zone).
export async function fetchTrainer(hour = new Date().getHours()) {
  return readJSON(await fetch(`/api/trainer?hour=${encodeURIComponent(hour)}`));
}

// Tells the trainer that a puzzle was done as practice ("По темам"), so that the plan can count it.
// Never throws: the puzzle page must not break because the plan could not be told.
export async function recordPractice(puzzle, solved) {
  try {
    await fetch("/api/trainer/practice", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ puzzle, solved: !!solved }),
    });
  } catch {
    // offline: the bar of the plan just does not move
  }
}

// Makes the puzzle page open on the theme (it reads the choice from the browser's storage).
export function rememberTheme(theme, storage = typeof localStorage === "undefined" ? null : localStorage) {
  if (!theme) return;
  try {
    storage?.setItem(PUZZLE_THEME_KEY, theme);
  } catch {
    // a private window: the page falls back to the first theme
  }
}

// ---------------------------------------------------------------- plan

// The share of a plan item that is done, 0..100; null for a point that cannot be measured.
export function itemPercent(item) {
  if (!item || !item.goal) return null;
  return Math.max(0, Math.min(100, Math.round((100 * item.progress) / item.goal)));
}

// "3 из 5".
export function progressText(item) {
  return item.goal ? tr("trainerJs.2", { progress: item.progress, goal: item.goal }) : "";
}

// The words under the day's bar: "Выполнено 2 из 3" or, when the plan is empty, "".
export function dayText(day) {
  if (!day || !day.total) return "";
  return day.done === day.total ? tr("trainerJs.3", { done: day.done, total: day.total }) : tr("trainerJs.4", { done: day.done, total: day.total });
}

export function dayPercent(day) {
  return day && day.total ? Math.round((100 * day.done) / day.total) : 0;
}

const KIND_ICONS = { puzzles: "puzzles", repertoire: "openings", review: "review", play: "bots", lesson: "lessons" };
export function kindIcon(kind) {
  return KIND_ICONS[kind] || "star";
}

// ---------------------------------------------------------------- how you play

// Columns of the stages of the game: the height in percent of the plot (0..100 accuracy, a floor of
// 4 so that a very low column is still seen) and the kind: "best", "weak" (trails the best by
// PHASE_GAP or more), "mid", or "none" for a stage with too few moves.
export function phaseBars(phases) {
  const known = (phases || []).filter((p) => p.accuracy != null);
  const best = known.length ? Math.max(...known.map((p) => p.accuracy)) : null;
  const worst = known.length ? Math.min(...known.map((p) => p.accuracy)) : null;
  return (phases || []).map((p) => {
    let kind = "none";
    if (p.accuracy != null) {
      kind = "mid";
      if (known.length > 1 && best - worst >= PHASE_GAP) {
        if (p.accuracy === best) kind = "best";
        else if (p.accuracy === worst) kind = "weak";
      }
    }
    return {
      id: p.id,
      name: p.name,
      moves: p.moves,
      accuracy: p.accuracy,
      kind,
      height: p.accuracy == null ? 0 : Math.max(4, Math.min(100, p.accuracy)),
    };
  });
}

// "61%" or "мало ходов".
export function accuracyLabel(accuracy) {
  return accuracy == null ? tr("trainerJs.5") : `${num(accuracy)}%`;
}

// A number with a decimal comma: 61 -> "61", 88.4 -> "88,4".
export function num(x) {
  if (x == null || Number.isNaN(Number(x))) return "—";
  return String(Math.round(x * 10) / 10).replace(".", getLanguage() === "en" ? "." : ",");
}

// Tiles with the slips per 100 moves.
export function rateTiles(rate) {
  return [
    { id: "inaccuracies", label: tr("trainerJs.6"), value: num(rate?.inaccuracies ?? 0), tone: "warn" },
    { id: "mistakes", label: tr("trainerJs.7"), value: num(rate?.mistakes ?? 0), tone: "mid" },
    { id: "blunders", label: tr("trainerJs.8"), value: num(rate?.blunders ?? 0), tone: "bad" },
  ];
}

// The tone of the trend: "up" for a rise of a point or more, "down" for a drop of a point, else "flat".
export function trendTone(trend) {
  if (!trend) return "flat";
  if (trend.delta >= 1) return "up";
  if (trend.delta <= -1) return "down";
  return "flat";
}

// The label of a costly move: "7.Ng5" for White's and "7...Nf6" for Black's.
export function moveLabel(c) {
  return c.black ? `${c.number}...${c.san}` : `${c.number}.${c.san}`;
}

const CLASS_WORDS = { inaccuracy: "Неточность", mistake: "Ошибка", blunder: "Зевок" };
export function classWord(cls) {
  return tx(CLASS_WORDS[cls] || "Ошибка");
}

const monthName = (i) => tr("stats.month." + (i + 1));
// "18 окт" in the viewer's time zone.
export function shortDate(iso) {
  const t = new Date(iso);
  return Number.isNaN(t.getTime()) ? "" : `${t.getDate()} ${monthName(t.getMonth())}`;
}

// "соперник Лиза, 18 окт".
export function costlyCaption(c) {
  return [c.opponent ? tr("trainerJs.24", { opponent: c.opponent }) : "", shortDate(c.at)].filter(Boolean).join(", ");
}

// ---------------------------------------------------------------- the chart of the dynamics

// Geometry of the accuracy line (reviews oldest first, average: the dashed line or null) in a width x height box. The scale is cut
// to a window around the data (multiples of 10, at least 30 points tall) so that small moves are seen.
export function dynamicsChart(series, average, width, height) {
  const pad = { left: 34, right: 14, top: 12, bottom: 22 };
  const empty = { points: [], line: "", ticks: [], avgY: null, pad };
  if (!series || series.length === 0 || width <= pad.left + pad.right) return empty;
  const values = series.map((p) => p.accuracy);
  let lo = Math.max(0, Math.floor((Math.min(...values) - 5) / 10) * 10);
  let hi = Math.min(100, Math.ceil((Math.max(...values) + 5) / 10) * 10);
  while (hi - lo < 30) {
    if (hi < 100) hi += 10;
    if (hi - lo < 30 && lo > 0) lo -= 10;
  }
  const w = width - pad.left - pad.right;
  const h = height - pad.top - pad.bottom;
  const y = (v) => pad.top + h * (1 - (v - lo) / (hi - lo));
  const x = (i) => (series.length === 1 ? pad.left + w / 2 : pad.left + (w * i) / (series.length - 1));
  const points = series.map((p, i) => ({ ...p, x: round1(x(i)), y: round1(y(p.accuracy)) }));
  const ticks = [];
  for (let v = lo; v <= hi; v += 10) ticks.push({ value: v, y: round1(y(v)) });
  return { points, line: points.map((p, i) => `${i ? "L" : "M"}${p.x} ${p.y}`).join(" "), ticks, avgY: average == null ? null : round1(y(average)), pad };
}

function round1(n) {
  return Math.round(n * 10) / 10;
}

// ---------------------------------------------------------------- notes

// Why some reviews were left out, in the person's words; "" when none were.
export function skippedNote(insights) {
  const n = insights?.skipped || 0;
  if (!n) return "";
  return tr("trainer.skippedNote", { n });
}

// How far the person is from the picture: "Разобрано 1 из 3" for the empty state.
export function needText(insights) {
  const need = insights?.need || 3;
  return tr("trainerJs.29", { p: Math.min(insights?.used || 0, need), need });
}

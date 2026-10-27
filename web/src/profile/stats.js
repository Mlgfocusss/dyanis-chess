// Pure helpers of the statistics tab: words, numbers and the geometry of the charts. No React and no
// imports from the page, so `npm test` covers them.
import { themeName } from "../puzzles/themes.js";
import { tr } from "../i18n/index.js";

// "1 день", "2 дня", "5 дней".
export function plural(n, one, few, many) {
  const m10 = Math.abs(n) % 10;
  const m100 = Math.abs(n) % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

export function daysText(n) {
  return tr("common.days", { n });
}

// The small tag next to the name in the profile head; "" when there is no streak.
export function streakBadge(streak) {
  const n = streak?.current || 0;
  return n > 0 ? tr("stats.4", { n }) : "";
}

// A share as a whole percent, null when there is nothing to divide.
export function percent(part, total) {
  return total > 0 ? Math.round((100 * part) / total) : null;
}

export function percentText(part, total) {
  const p = percent(part, total);
  return p === null ? "—" : `${p}%`;
}

// Widths (in percent, adding up to 100) of the win / draw / loss bar; all zero without games.
export function recordBar(rec) {
  const total = rec?.total || 0;
  if (!total) return { win: 0, draw: 0, loss: 0 };
  const win = Math.round((100 * rec.wins) / total);
  const loss = Math.round((100 * rec.losses) / total);
  return { win, draw: Math.max(0, 100 - win - loss), loss };
}

export function recordText(rec) {
  return `${rec?.wins || 0} / ${rec?.draws || 0} / ${rec?.losses || 0}`;
}

// "+10", "−16", "±0" (a real minus sign, like the puzzle pages).
export function signedDelta(n) {
  if (n > 0) return `+${n}`;
  if (n < 0) return `−${Math.abs(n)}`;
  return "±0";
}

// Which sentence tells how the accuracy was taken. known is the number of reviews in which the
// person's colour was recognised by name, sample the number of reviews taken.
export function accuracyNote(reviews) {
  const sample = reviews?.sample || 0;
  const known = reviews?.known || 0;
  if (!sample) return "";
  if (known === sample) return tr("stats.5");
  if (known === 0) {
    return tr("stats.6");
  }
  return tr("stats.7", { known, sample });
}

const SIDE_WORDS = { white: "stats.sideWhite", black: "stats.sideBlack", both: "stats.sideBoth" };
export function sideText(side) {
  return tr(SIDE_WORDS[side] || SIDE_WORDS.both);
}

// ---------------------------------------------------------------- accuracy chart

// Geometry of the accuracy line in a width x height box with room for axis labels. The scale is
// 0..100 cut to a window around the data (multiples of 10, at least 30 points tall) so that small
// moves stay visible. Returns the points, the path of the line, the grid lines and the mean line.
export function accuracyChart(series, average, width, height) {
  const pad = { left: 34, right: 14, top: 12, bottom: 22 };
  const empty = { points: [], line: "", ticks: [], avgY: null, pad };
  if (!series || series.length === 0 || width <= pad.left + pad.right) return empty;

  const values = series.map((p) => p.accuracy);
  let lo = Math.floor((Math.min(...values) - 5) / 10) * 10;
  let hi = Math.ceil((Math.max(...values) + 5) / 10) * 10;
  lo = Math.max(0, lo);
  hi = Math.min(100, hi);
  while (hi - lo < 30) {
    if (hi < 100) hi += 10;
    if (hi - lo < 30 && lo > 0) lo -= 10;
  }
  const w = width - pad.left - pad.right;
  const h = height - pad.top - pad.bottom;
  const y = (v) => pad.top + h * (1 - (v - lo) / (hi - lo));
  const x = (i) => (series.length === 1 ? pad.left + w / 2 : pad.left + (w * i) / (series.length - 1));

  const points = series.map((p, i) => ({ ...p, x: round(x(i)), y: round(y(p.accuracy)) }));
  const line = points.map((p, i) => `${i ? "L" : "M"}${p.x} ${p.y}`).join(" ");
  const ticks = [];
  for (let v = lo; v <= hi; v += 10) ticks.push({ value: v, y: round(y(v)) });
  return { points, line, ticks, avgY: average == null ? null : round(y(average)), pad };
}

function round(n) {
  return Math.round(n * 10) / 10;
}

// ---------------------------------------------------------------- activity strip

const monthName = (i) => tr("stats.month." + (i + 1));

// "12 окт" for "2026-10-12".
export function dayLabel(iso) {
  const [, m, d] = String(iso).split("-").map(Number);
  return m >= 1 && m <= 12 && d ? `${d} ${monthName(m - 1)}` : String(iso);
}

export function puzzlesText(n) {
  return tr("stats.puzzlesN", { n });
}

// One cell per day of the streak window: its date, count, intensity level 0..4 and a caption.
// Levels split the busiest day of the window in quarters, so a quiet profile still shows colour.
export function activityCells(streak) {
  const counts = streak?.days || [];
  if (!counts.length || !streak.from) return [];
  const [y, m, d] = streak.from.split("-").map(Number);
  const max = Math.max(...counts);
  return counts.map((count, i) => {
    const date = new Date(Date.UTC(y, m - 1, d + i)).toISOString().slice(0, 10);
    const level = count <= 0 ? 0 : Math.min(4, Math.ceil((4 * count) / max));
    const label = `${dayLabel(date)}: ${count > 0 ? puzzlesText(count) : tr("stats.26")}`;
    return { date, count, level, label };
  });
}

export function activeDays(streak) {
  return (streak?.days || []).filter((n) => n > 0).length;
}

// ---------------------------------------------------------------- tables

// Rows of the weak-themes table: the Russian name, the failures and the share of failures.
export function weakRows(weak) {
  return (weak || []).map((t) => ({
    theme: t.theme,
    name: themeName(t.theme),
    attempts: t.attempts,
    failed: t.failed,
    share: percent(t.failed, t.attempts) ?? 0,
  }));
}

// "сегодня, 14:05", "вчера, 09:30" or "12 окт, 18:42" in the viewer's time zone.
export function whenText(iso, now = new Date()) {
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return "";
  const clock = `${String(t.getHours()).padStart(2, "0")}:${String(t.getMinutes()).padStart(2, "0")}`;
  const startOf = (x) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const days = Math.round((startOf(now) - startOf(t)) / 86400000);
  if (days === 0) return tr("stats.27", { clock });
  if (days === 1) return tr("stats.28", { clock });
  return `${t.getDate()} ${monthName(t.getMonth())}, ${clock}`;
}

// Rows of the history of tries: the link opens the puzzle (#puzzles/<id>).
export function historyRows(history, now = new Date()) {
  return (history || []).map((a) => ({
    id: a.puzzle,
    href: `#puzzles/${encodeURIComponent(a.puzzle)}`,
    solved: a.solved,
    rating: a.after,
    delta: signedDelta(a.delta),
    when: whenText(a.at, now),
  }));
}

// "3 мин: 14 (заездов 2)" per mode of the tactics storm, best first by mode order.
const RUSH_NAMES = { "3min": "stats.rush3", "5min": "stats.rush5" };
export function rushRows(rush) {
  return Object.keys(RUSH_NAMES)
    .filter((k) => rush?.[k])
    .map((k) => ({ mode: k, name: tr(RUSH_NAMES[k]), best: rush[k].best, runs: rush[k].runs }));
}

// True when nothing at all has been played yet.
export function isEmpty(stats) {
  return !stats || (!stats.games.total && !stats.reviews.count && !stats.puzzles.attempts && !stats.streak.best);
}

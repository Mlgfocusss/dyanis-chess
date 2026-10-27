import { tr } from "../i18n/index.js";
// Clock text and the list of time controls. Plain JavaScript, covered by
// `npm test`.

// 605000 -> "10:05"; under a minute "0:42"; under ten seconds with tenths,
// "0:09.4"; an hour or more "1:02:03".
export function formatClock(ms) {
  const t = Math.max(0, Math.ceil(ms / 100)); // tenths, rounded up so 0:00.0 means really out
  const s = Math.floor(t / 10);
  if (s < 10) return `0:0${s}.${t % 10}`;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${sec}`;
  return `${m}:${sec}`;
}

// What a game's time control is called: "5 + 2", "10 мин", "без часов".
export function describeTC(tc) {
  if (tc?.days) return tr("clock.corr", { n: tc.days });
  if (!tc || !tc.baseMs) return tr("clock.2");
  const whole = Number.isInteger(tc.baseMs / 60000);
  const base = whole ? String(tc.baseMs / 60000) : tr("clock.3", { p: tc.baseMs / 1000 });
  if (tc.incMs) return `${base} + ${tc.incMs / 1000}`;
  return whole ? tr("clock.4", { base }) : base;
}

// Groups of ready choices for the lobby: [minutes, increment seconds]. group is a dictionary key.
export const TIME_CONTROLS = [
  { group: "rating.cat.bullet", options: [[1, 0], [2, 1]] },
  { group: "rating.cat.blitz", options: [[3, 0], [3, 2], [5, 0]] },
  { group: "rating.cat.rapid", options: [[10, 0], [10, 5], [15, 10]] },
  { group: "clock.classical", options: [[30, 0]] },
];

export const tcToRequest = ([minutes, inc]) => ({ timeMs: minutes * 60000, incMs: inc * 1000 });

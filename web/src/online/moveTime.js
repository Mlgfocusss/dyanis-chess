// The time a move took, written short, and the figures the review shows about it. Plain JavaScript with
// no imports, so it is covered by `npm test`.

// What the units are called; the caller passes the words of the current language.
export const RU_UNITS = { s: "с", m: "м", h: "ч", d: "д", dec: "," };

// 2300 -> "2,3 с"; 23000 -> "23 с"; 65000 -> "1:05"; 1h20m -> "1 ч 20 м"; 2d3h -> "2 д 3 ч".
// null or a negative number (unknown) -> "".
export function formatMoveTime(ms, u = RU_UNITS) {
  if (ms === null || ms === undefined || !Number.isFinite(ms) || ms < 0) return "";
  if (ms < 9950) {
    const tenths = Math.round(ms / 100);
    if (tenths === 0) return `0 ${u.s}`;
    const s = String(tenths / 10);
    return `${(s.includes(".") ? s : `${s}.0`).replace(".", u.dec)} ${u.s}`;
  }
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec} ${u.s}`;
  if (sec < 3600) return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, "0")}`;
  const mins = Math.floor(sec / 60);
  if (mins < 24 * 60) {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    return m ? `${h} ${u.h} ${m} ${u.m}` : `${h} ${u.h}`;
  }
  const hours = Math.floor(mins / 60);
  const d = Math.floor(hours / 24);
  const h = hours % 24;
  return h ? `${d} ${u.d} ${h} ${u.h}` : `${d} ${u.d}`;
}

const known = (m) => m && typeof m.timeMs === "number" && m.timeMs >= 0;

// Does the report carry move times at all (a game without clocks, or an old report, has none)?
export function hasMoveTimes(moves) {
  return (moves || []).some(known);
}

// The average time of a move of each side, in ms, or null where there is nothing to average.
export function averageTimes(moves) {
  const sum = { white: 0, black: 0 };
  const n = { white: 0, black: 0 };
  for (const m of moves || []) {
    if (!known(m) || (m.color !== "white" && m.color !== "black")) continue;
    sum[m.color] += m.timeMs;
    n[m.color]++;
  }
  return {
    white: n.white ? sum.white / n.white : null,
    black: n.black ? sum.black / n.black : null,
  };
}

// The plies of the `count` longest moves, longest first; moves under `min` ms are not worth a mention.
export function longestMoves(moves, count = 3, min = 3000) {
  return (moves || [])
    .filter((m) => known(m) && m.timeMs >= min)
    .sort((a, b) => b.timeMs - a.timeMs || a.ply - b.ply)
    .slice(0, count)
    .map((m) => m.ply);
}

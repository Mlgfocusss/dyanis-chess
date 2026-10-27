// What the builder suggests at a position, with no imports of its own so that
// `npm test` covers it (it uses the pure helpers of the review page).
import { moverLoss } from "../review/branch.js";
import { getLanguage, tr } from "../i18n/index.js";

// A move that loses this many win-probability points against the position's own
// value is shown as weak (the same cut as a mistake in the review).
export const WEAK_LOSS = 10;
// A theory move with fewer lines behind it than this share of the best known
// one is shown as rare.
export const RARE_SHARE = 0.05;
// With the statistics of real games: a move played in fewer than this share of them is rare.
export const RARE_GAMES_SHARE = 0.01;

// The rating ranges of the statistics, as the Lichess explorer groups the players
// (the explorer takes the lower bounds of its groups).
export const RATING_PRESETS = [
  { id: "low", label: "suggest.presetLow", ratings: "0,1000,1200" },
  { id: "mid", label: "1400–1800", ratings: "1400,1600" },
  { id: "high", label: "1800–2200", ratings: "1800,2000" },
  { id: "top", label: "suggest.presetTop", ratings: "2200,2500" },
];
export const DEFAULT_PRESET = "mid";
const PRESET_KEY = "dyanis.explorerRating";

// The rating range the person chose last (kept in the browser), shared by the list of moves and the search for holes.
export function loadPreset() {
  try {
    const id = localStorage.getItem(PRESET_KEY);
    return RATING_PRESETS.some((p) => p.id === id) ? id : DEFAULT_PRESET;
  } catch {
    return DEFAULT_PRESET;
  }
}

export function storePreset(id) {
  try {
    localStorage.setItem(PRESET_KEY, id);
  } catch {
    // a private window: the choice is just not remembered
  }
}

export const ratingsOf = (id) => (RATING_PRESETS.find((p) => p.id === id) || RATING_PRESETS[1]).ratings;

// Rows of "moves in this position", from three sources:
//   children  the moves already in the repertoire at this position (tree moves)
//   theory    the moves opening theory knows ([{uci, san, opening, lines}])
//   best      the engine's choice for the position ({uci, san}), if it is known
//   stats     the Lichess statistics of the position ({available, total, moves: [{uci, san,
//             games, white, draws, black, averageRating, opening}]}) or null; when it has
//             moves, popularity is the share of real games, otherwise the lines of theory
// after: Map uci -> the engine's verdict on the position after that move (white's
// win chance in `winPct`), before: the verdict on the position itself, side:
// who moves ("white" or "black").
// Each row: { uci, san, repo, theory, engine, lines, share, opening, ev, loss, weak, rare }.
// The order: the repertoire's own moves first, then theory by how much stands
// behind it with the weak moves last, then the engine's own suggestion.
export function buildSuggestions({ children = [], theory = [], best = null, after = new Map(), before = null, side, stats = null }) {
  const rows = new Map();
  const row = (uci, san) => {
    if (!rows.has(uci)) rows.set(uci, { uci, san, repo: null, theory: false, engine: false, lines: 0, opening: null, stat: false, games: 0, white: 0, draws: 0, black: 0, averageRating: 0 });
    return rows.get(uci);
  };
  for (const m of children) row(m.uci, m.san).repo = m;
  for (const t of theory) {
    const r = row(t.uci, t.san);
    r.theory = true;
    r.lines = t.lines || 0;
    r.opening = t.opening || null;
  }
  const played = stats && stats.available && stats.moves && stats.moves.length > 0 && stats.total > 0 ? stats : null;
  if (played) {
    for (const m of played.moves) {
      const r = row(m.uci, m.san);
      Object.assign(r, { stat: true, games: m.games, white: m.white, draws: m.draws, black: m.black, averageRating: m.averageRating || 0 });
      r.opening = r.opening || m.opening || null;
    }
  }
  if (best && best.uci) row(best.uci, best.san).engine = true;

  const maxLines = Math.max(0, ...[...rows.values()].map((r) => r.lines));
  const out = [...rows.values()].map((r) => {
    const ev = after.get(r.uci) || null;
    const loss = ev ? moverLoss(before, ev, side) : null;
    return {
      ...r,
      ev,
      loss,
      share: played ? r.games / played.total : maxLines ? r.lines / maxLines : 0,
      weak: loss !== null && loss >= WEAK_LOSS,
      rare: played ? r.stat && r.games / played.total < RARE_GAMES_SHARE : r.theory && maxLines > 0 && r.lines / maxLines < RARE_SHARE,
    };
  });
  const group = (r) => (r.repo ? 0 : r.theory || r.stat ? 1 : 2);
  const popularity = (r) => (played ? r.games : r.lines);
  out.sort((a, b) => {
    if (group(a) !== group(b)) return group(a) - group(b);
    if (a.weak !== b.weak) return a.weak ? 1 : -1;
    if (popularity(a) !== popularity(b)) return popularity(b) - popularity(a);
    if (a.lines !== b.lines) return b.lines - a.lines; // equal in games: the better known theory first
    return a.san < b.san ? -1 : a.san > b.san ? 1 : 0;
  });
  return out;
}

// A run of evaluation requests with at most `limit` going at once, the rest
// waiting in order. A task is a function returning a promise; the queue gives
// back that promise.
export function makeQueue(limit) {
  let running = 0;
  const waiting = [];
  const next = () => {
    while (running < limit && waiting.length) {
      const { task, resolve, reject } = waiting.shift();
      running++;
      Promise.resolve()
        .then(task)
        .then(resolve, reject)
        .finally(() => {
          running--;
          next();
        });
    }
  };
  return (task) =>
    new Promise((resolve, reject) => {
      waiting.push({ task, resolve, reject });
      next();
    });
}

// "1,2 млн", "340 тыс.", "812": how many games, short enough for a row.
export function formatGames(n) {
  if (!(n > 0)) return "0";
  if (n >= 1e6) return tr("suggest.3", { p: (n / 1e6).toFixed(1).replace(".", getLanguage() === "en" ? "." : ",") });
  if (n >= 1e4) return tr("suggest.4", { p: Math.round(n / 1e3) });
  if (n >= 1e3) return tr("suggest.4", { p: (n / 1e3).toFixed(1).replace(".", getLanguage() === "en" ? "." : ",") });
  return String(n);
}

// A share as a percentage for a row: "42%", and "<1%" for a move that is played but rare.
export function formatShare(share) {
  const pct = share * 100;
  if (pct > 0 && pct < 1) return "<1%";
  return `${Math.round(pct)}%`;
}

// The results of the games of a move as shares: { white, draws, black } in 0..100, summing to 100.
export function resultShares(r) {
  const n = r.white + r.draws + r.black;
  if (!n) return { white: 0, draws: 0, black: 0 };
  const white = Math.round((100 * r.white) / n);
  const draws = Math.round((100 * r.draws) / n);
  return { white, draws, black: 100 - white - draws };
}

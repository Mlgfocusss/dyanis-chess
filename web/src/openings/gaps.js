// Holes ("дырки") of a repository, in the terms of the page: the popular replies of the opponent
// that the tree has no answer to, and the lines that end with the opponent's move. Pure logic with
// no imports of its own besides the language helpers, so that `npm test` covers it.
// The search itself is done by the server (backend/internal/repertoire/gaps_popular.go); a hole
// of its answer is { kind: "missing"|"answer", at, ply, fen, path, uci, san, pct, games, lines,
// weight, source }, where `at` is the node of the position the hole is shown in.
import { getLanguage } from "../i18n/index.js";

// What the server counts as a popular reply (the defaults of its search).
export const MIN_PCT = 5;
export const MIN_GAMES = 50;

export const keyOf = (g) => `${g.kind}:${g.at}:${g.uci || ""}`;

// "3. Nf3" / "3... c5": a move written for the number of plies already played before it.
export function moveLabel(ply, san) {
  const no = Math.floor(ply / 2) + 1;
  return ply % 2 === 0 ? `${no}. ${san}` : `${no}... ${san}`;
}

// The holes of the first kind that the browser can see by itself (lines that end with the
// opponent's move), in the shape of a server hole. Nothing is known of their weight.
export function localAnswerGaps(tree) {
  const out = [];
  for (const m of tree.byId.values()) {
    if (!m.mine && !(tree.children.get(m.id) || []).length) {
      out.push({ kind: "answer", at: m.id, ply: m.ply, fen: m.fen, uci: m.uci, san: m.san, pct: 0, games: 0, weight: null, source: "" });
    }
  }
  return out;
}

// A hole of the last search as the tree stands now, or null when the tree has closed it. The tree may have
// changed since the search (a move added, a line deleted): a missing reply the person has since added
// becomes the same line without an answer (it keeps its weight), one that has an answer is closed.
function current(g, tree) {
  if (g.at !== 0 && !tree.byId.has(g.at)) return null;
  const kids = tree.children.get(g.at) || [];
  if (g.kind === "answer") return kids.length === 0 ? g : null;
  const added = kids.find((m) => m.uci === g.uci);
  if (!added) return g;
  if ((tree.children.get(added.id) || []).length) return null;
  return { ...g, kind: "answer", at: added.id, ply: added.ply, fen: added.fen };
}

// The holes to show: those of the last search that still stand, plus the unanswered lines the
// search did not list (it lists the heaviest only, and it may be older than the tree), the
// heaviest first; holes of unknown weight go last. An empty repository of White has no line to
// call a hole: the page says "add the first move" instead.
export function visibleGaps(scan, tree) {
  const seen = new Set();
  const out = [];
  for (const g of scan?.gaps || []) {
    if (g.kind === "answer" && g.at === 0) continue;
    const now = current(g, tree);
    if (!now || seen.has(keyOf(now))) continue;
    seen.add(keyOf(now));
    out.push(now);
  }
  for (const g of localAnswerGaps(tree)) {
    if (!seen.has(keyOf(g))) out.push(g);
  }
  const w = (g) => (g.weight == null ? -1 : g.weight);
  out.sort((a, b) => w(b) - w(a) || a.ply - b.ply || (a.uci < b.uci ? -1 : a.uci > b.uci ? 1 : 0));
  return out;
}

// The nodes of the tree to mark with a dot: where a hole is shown.
export function markedNodes(gaps) {
  const out = new Set();
  for (const g of gaps) if (g.at) out.add(g.at);
  return out;
}

// The hole after the one in focus, going round the list; the first when nothing is in focus.
export function nextGap(gaps, focusKey) {
  if (!gaps.length) return null;
  const i = gaps.findIndex((g) => keyOf(g) === focusKey);
  return gaps[(i + 1) % gaps.length];
}

// The figures of a list of holes.
export function gapCounts(gaps) {
  let missing = 0;
  for (const g of gaps) if (g.kind === "missing") missing++;
  return { missing, answer: gaps.length - missing, total: gaps.length };
}

// 0..100 for a coverage of 0..1; never 100 while something is open and never 0 once started, so
// that the bar does not lie at its ends.
export function coveragePercent(coverage, openHoles = 0) {
  const p = Math.round(100 * (Number.isFinite(coverage) ? coverage : 0));
  return openHoles > 0 ? Math.min(p, 99) : p;
}

// A chance as a percentage for a row: "37%", "0,8%", "<0,1%".
export function formatWeight(w) {
  if (w == null || !Number.isFinite(w)) return "";
  const pct = w * 100;
  if (pct <= 0) return "0%";
  if (pct < 0.1) return "<0.1%".replace(".", getLanguage() === "en" ? "." : ",");
  if (pct < 10) return `${pct.toFixed(1).replace(".", getLanguage() === "en" ? "." : ",")}%`;
  return `${Math.round(pct)}%`;
}

// What kind of statistics stand behind a search: "lichess", "theory" (the reference, no
// statistics), "mixed", or "" when there was none.
export function sourceOf(scan) {
  if (!scan) return "";
  return scan.source === "lichess" || scan.source === "mixed" ? scan.source : "theory";
}

// Whether a row of "moves in this position" (see suggest.js) is a popular reply of the opponent that
// the repository lacks: the same rule as the server's. hasStats: the rows carry real games.
export function isPopularReply(row, hasStats) {
  if (row.repo) return false;
  if (hasStats) return row.stat && row.games >= MIN_GAMES && row.share * 100 >= MIN_PCT;
  return row.theory && row.share * 100 >= MIN_PCT;
}

// A search request is wanted when the tree is not what the last answer saw.
export function treeSignature(moves) {
  let max = 0;
  for (const m of moves) if (m.id > max) max = m.id;
  return `${moves.length}:${max}`;
}

// What a card of the list says about holes, from the summary the list endpoint gives without asking
// Lichess ({open, scanned, missing, coverage, partial, stale, source}): the lines without an answer
// are counted afresh each time, the popular replies the tree lacks only by the last search.
//   holes     how many holes the person can be told about
//   checked   whether the last search is there at all (otherwise only the open lines are known)
//   coverage  0..100 or null when no search was made
export function cardHoles(g) {
  if (!g) return { holes: 0, checked: false, stale: false, partial: false, coverage: null, source: "" };
  const holes = g.open + (g.scanned ? g.missing : 0);
  return {
    holes,
    checked: !!g.scanned,
    stale: !!g.stale,
    partial: !!g.partial,
    coverage: g.scanned ? coveragePercent(g.coverage, holes) : null,
    source: g.source || "",
  };
}

// The strip on top of the list: repositories, moves, holes, cards to repeat today (and new ones).
export function listTotals(reps) {
  const out = { reps: reps.length, moves: 0, holes: 0, due: 0, fresh: 0 };
  for (const r of reps) {
    out.moves += r.counts.moves;
    out.holes += cardHoles(r.gaps).holes;
    out.due += r.counts.due;
    out.fresh += r.counts.new;
  }
  return out;
}

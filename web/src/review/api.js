import { serverMsg, tr } from "../i18n/index.js";
// Client of the review API (internal/api).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    throw new Error(serverMsg(body?.error) || tr("common.serverReplied", { status: res.status }));
  }
  return body;
}

export async function createAnalysis({ pgn, game = 1, depth = 0 }) {
  let res;
  try {
    res = await fetch("/api/analyses", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pgn, game, depth }),
    });
  } catch {
    throw new Error(tr("reviewApi.2"));
  }
  return (await readJSON(res)).id;
}

export async function getAnalysis(id) {
  return readJSON(await fetch(`/api/analyses/${encodeURIComponent(id)}`));
}

// The signed-in user's reviews, newest first (rows of api.Listed).
export async function listAnalyses() {
  return (await readJSON(await fetch("/api/analyses"))).items;
}

export async function deleteAnalysis(id) {
  const res = await fetch(`/api/analyses/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!res.ok) await readJSON(res); // throws with the server's message
}

// Plays moves (UCI or SAN) from a position on the server, which knows the rules:
// { steps: [{uci, san, fen}], fen, legal: [uci], status, eval? }. With eval the
// engine also judges the last position (depth 0 = server default); with lines (up to 3) it also
// finds that many best lines of it (MultiPV): lines: [{rank, score, mate, winPct, depth, bestSan, pv}].
export async function fetchLine({ fen, moves = [], eval: wantEval = false, depth = 0, lines = 0 }) {
  const res = await fetch("/api/line", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fen, moves, eval: wantEval, depth, lines }),
  });
  return readJSON(res);
}

// Whether the AI coach is switched on at the server (it needs an OpenRouter key). Asked once.
let coachStatus = null;
export function coachEnabled() {
  coachStatus ??= fetch("/api/coach")
    .then((r) => (r.ok ? r.json() : { enabled: false }))
    .then((b) => !!b.enabled)
    .catch(() => false);
  return coachStatus;
}

// The coach's explanation of a move of a review (ply: the 1-based half-move):
// { text, cached, bestSan, bestLine: [san], replyLine: [san] }.
export async function askCoach(id, ply) {
  const res = await fetch("/api/coach", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id, ply }),
  });
  return readJSON(res);
}

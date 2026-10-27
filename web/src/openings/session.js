import { tr } from "../i18n/index.js";
// The queue of a training session, with no imports so that `npm test` covers it.
//
// A session is a list of drills (see backend/internal/repertoire: Drill). Every
// drill is asked until it has been answered right at the first try:
//   - a new move is first shown and repeated, then comes back at the end of the
//     session as a question;
//   - a move answered wrong (or given up, or helped by a hint) comes back after a
//     few other questions.
// Only the first result of a move in the session is sent to the server, so that
// asking it again does not climb the ladder twice.

// How many other questions go between a failed one and its return.
export const RETRY_GAP = 3;

// entry: { drill, show, recall, retry }
export function startQueue(drills) {
  return drills.map((drill) => ({ drill, show: !!drill.new, recall: false, retry: false }));
}

// What the person's move `uci` means in a drill: "right", "alt" (another move of
// the repertoire in this position, not a mistake but not the one asked) or "wrong".
export function judge(drill, uci) {
  if (uci === drill.answer.uci) return "right";
  if ((drill.alt || []).includes(uci)) return "alt";
  return "wrong";
}

// The queue after the first entry has been dealt with. outcome:
//   "shown"  a new move was shown and repeated
//   "right"  answered right at the first try
//   "wrong"  a mistake, a hint or giving up came first
export function advance(queue, outcome) {
  const [head, ...rest] = queue;
  if (!head) return queue;
  if (outcome === "right") return rest;
  if (outcome === "shown") return [...rest, { ...head, show: false, recall: true, retry: false }];
  const again = { ...head, show: false, recall: false, retry: true };
  const at = Math.min(rest.length, RETRY_GAP);
  return [...rest.slice(0, at), again, ...rest.slice(at)];
}

// What to tell the server about an outcome: true (right), false (failed) or
// null (nothing: a move that was only shown, or one already reported).
export function reportFor(reported, entry, outcome) {
  if (outcome === "shown" || reported.has(entry.drill.move)) return null;
  return outcome === "right";
}

// The numbers of the end screen: how many moves were right at the first try.
export function summary(results) {
  const values = Object.values(results);
  return { total: values.length, right: values.filter((v) => v === true).length, wrong: values.filter((v) => v === false).length };
}

// How to answer a move that is not the one being learned. Both verdicts are the
// engine's on the position after the move (white's chance in `winPct`): the one
// the person played and the one the repertoire wants; side is who moves.
//   fine        about as good as the repertoire's move: "good, but we learn another branch"
//   inaccurate  a little worse
//   blunder     a gross mistake
// loss is the win-probability points the person's move gives up against the
// repertoire's (0 when it is better). A missing verdict gives { kind: "unknown" }.
export const DEVIATION_FINE = 5;
export const DEVIATION_BLUNDER = 15;

export function deviationVerdict(evAnswer, evPlayed, side) {
  if (!evAnswer || !evPlayed || typeof evAnswer.winPct !== "number" || typeof evPlayed.winPct !== "number") return { kind: "unknown", loss: null };
  const sign = side === "white" ? 1 : -1;
  const loss = Math.max(0, sign * (evAnswer.winPct - evPlayed.winPct));
  const kind = loss >= DEVIATION_BLUNDER ? "blunder" : loss >= DEVIATION_FINE ? "inaccurate" : "fine";
  return { kind, loss };
}

// The sentence for the page: playedSan is what the person played, answerSan what is learned.
export function deviationText(verdict, playedSan, answerSan) {
  const loss = verdict.loss === null ? 0 : Math.round(verdict.loss);
  switch (verdict.kind) {
    case "fine":
      return tr("session.1", { playedSan, answerSan });
    case "inaccurate":
      return tr("session.2", { playedSan, loss, answerSan });
    case "blunder":
      return tr("session.3", { playedSan, loss, answerSan });
    default:
      return tr("session.4", { answerSan });
  }
}

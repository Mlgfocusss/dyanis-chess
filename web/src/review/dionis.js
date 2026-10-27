// Dionis, the coach of the platform: a short remark on each move of a reviewed game, made by rules
// from what the review already knows (the class of the move, the engine's best move, the reply that
// punishes it). Only plain-JS imports, so that `npm test` covers it. The longer, AI-written explanation is a
// separate thing (CoachCard in DionisCard.jsx).

import { openingName } from "../openings/names.js";
import { tx, tr } from "../i18n/index.js";

const VICTIM = { p: "пешку", n: "коня", b: "слона", r: "ладью", q: "ферзя" };

// Moves that cost something: the coach names what was lost and what to play instead.
const COSTLY = new Set(["inaccuracy", "mistake", "blunder", "miss"]);
export const isCostly = (cls) => COSTLY.has(cls);

// The piece (lower case letter) on a square of a FEN, "" when empty.
export function pieceAt(fen, sq) {
  if (!fen || !/^[a-h][1-8]$/.test(sq || "")) return "";
  const row = String(fen).split(" ")[0].split("/")[8 - Number(sq[1])];
  if (!row) return "";
  let file = 0;
  for (const c of row) {
    if (c >= "1" && c <= "8") file += Number(c);
    else {
      if (file === sq.charCodeAt(0) - 97) return c.toLowerCase();
      file++;
    }
  }
  return "";
}

// The square a move in SAN goes to ("Qxg5+" -> "g5", "exd6" -> "d6", "e8=Q" -> "e8"); null for castling.
export function targetSquare(san) {
  const m = /([a-h][1-8])(?:=[QRBN])?[+#]?$/.exec(String(san || ""));
  return m ? m[1] : null;
}

// What a move in SAN takes, as the thing taken in the accusative ("коня"), or "" when it takes
// nothing. fen is the position BEFORE the move (an en passant capture finds an empty square: a pawn).
export function takes(san, fen) {
  if (!String(san || "").includes("x")) return "";
  const piece = pieceAt(fen, targetSquare(san));
  return (VICTIM[piece] ? tx(VICTIM[piece]) : "") || tr("dionis.1");
}

const pick = (list, n) => list[Math.abs(n) % list.length];

// The moves to a mate that the position after the move holds for the side that did NOT move
// (what the move allows), or 0.
function mateAllowed(move, after) {
  if (!after || !after.mate) return 0;
  const n = move.color === "white" ? -after.mate : after.mate;
  return n > 0 ? n : 0;
}

export const GREETING = "Привет, я Дионис. Листайте партию стрелками: я коротко объясню каждый ход.";

// before: the engine's verdict on the position before the move, after: on the position after it
// (the review's evals[ply-1] and evals[ply]); both may be missing.
// Returns { text, threat }: threat means the engine's move in `after` is the reply that punishes
// the move, to be drawn as a red arrow; best is the engine's move instead, when it differs.
export function brief({ move, before, after }) {
  if (!move) return { text: tr("dionis.greeting"), threat: false, best: "" };
  const cls = move.class;
  const best = move.bestSan && move.bestSan !== move.san ? move.bestSan : "";
  const n = move.ply || 0;
  const terminal = !!after?.terminal;
  const reply = !terminal && after?.bestSan ? after.bestSan : "";
  const lost = reply ? takes(reply, after.fen) : "";
  const mate = terminal ? 0 : mateAllowed(move, after);
  const instead = best ? ` ${cls === "mistake" || cls === "blunder" ? tr("dionis.7") : tr("dionis.8")} ${best}.` : "";

  // what the opponent's best reply does, in a few words
  const punishment = () => {
    if (mate > 0 && mate <= 6) return tr("dionis.9", { mate });
    if (lost) return tr("dionis.10", { reply, lost });
    return "";
  };

  let text;
  switch (cls) {
    case "brilliant":
      text = pick([tr("dionis.11"), tr("dionis.12")], n);
      break;
    case "great":
      text = tr("dionis.13");
      break;
    case "book": {
      const name = move.opening ? openingName(move.opening) : "";
      text = name ? tr("dionis.14", { name }) : tr("dionis.15");
      break;
    }
    case "best": {
      const won = takes(move.san, before?.fen);
      if (move.san.endsWith("#")) text = tr("dionis.16");
      else if (move.forced) text = tr("dionis.17");
      else if (won) text = tr("dionis.18", { won });
      else text = pick([tr("dionis.19"), tr("dionis.20"), tr("dionis.21")], n);
      break;
    }
    case "excellent":
      text = pick([tr("dionis.22"), tr("dionis.23")], n);
      break;
    case "good":
      text = best ? tr("dionis.24", { best }) : tr("dionis.25");
      break;
    case "inaccuracy": {
      const p = punishment();
      text = p ? tr("dionis.26", { p, instead }) : tr("dionis.27", { instead });
      break;
    }
    case "mistake": {
      const p = punishment();
      text = p ? tr("dionis.28", { p, instead }) : tr("dionis.29", { instead });
      break;
    }
    case "blunder": {
      const p = punishment();
      text = p ? tr("dionis.30", { toUpperCase: p[0].toUpperCase(), p: p.slice(1), instead }) : tr("dionis.31", { instead });
      break;
    }
    case "miss":
      text = best ? tr("dionis.32", { best }) : tr("dionis.33");
      break;
    default:
      text = "";
  }
  return { text, threat: isCostly(cls) && !!reply, best };
}

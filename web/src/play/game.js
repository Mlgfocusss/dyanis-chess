// Pure helpers for the end of a game against the bot: what the result is,
// how to say it, and the PGN to send to the review. No imports, so they are
// covered by `npm test`.

export const START_FEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

const SIDE_NAME = { white: "Белые", black: "Чёрные" };

// state: the engine's state ({status, sideToMove}); resigned: "white" |
// "black" | null (who gave up); playerSide: the human's colour. Returns null
// while the game is going on, else:
//   result   PGN result: "1-0", "0-1" or "1/2-1/2"
//   outcome  "win" | "loss" | "draw" from the human's point of view
//   reason   short cause: "Мат", "Сдача", "Пат", ...
//   title    headline: "Вы победили!", "Вы проиграли", "Ничья"
//   subtitle the same in words about colours: "Белые победили матом"
export function describeResult({ status, sideToMove, resigned, playerSide }) {
  let winner = null; // "white" | "black" | null for a draw
  let reason;
  let how; // "матом" / "по сдаче" for the subtitle
  if (resigned) {
    winner = resigned === "white" ? "black" : "white";
    reason = "Сдача";
    how = "по сдаче соперника";
  } else if (status === "checkmate") {
    winner = sideToMove === "white" ? "black" : "white";
    reason = "Мат";
    how = "матом";
  } else if (status === "stalemate") {
    reason = "Пат";
  } else if (status === "draw-fifty-move") {
    reason = "Правило 50 ходов";
  } else if (status === "draw-repetition") {
    reason = "Троекратное повторение";
  } else {
    return null;
  }

  if (!winner) {
    return { result: "1/2-1/2", outcome: "draw", reason, title: "Ничья", subtitle: `Ничья: ${reason.toLowerCase()}` };
  }
  const outcome = winner === playerSide ? "win" : "loss";
  return {
    result: winner === "white" ? "1-0" : "0-1",
    outcome,
    reason,
    title: outcome === "win" ? "Вы победили!" : "Вы проиграли",
    subtitle: `${SIDE_NAME[winner]} победили ${how}`,
  };
}

// The engine's game log ("1. e4 e5 2. Nf3") without the move numbers. The
// numbers are dropped because the engine numbers from 1 and white even for
// a game that began from a position with Black to move.
export function sanMoves(log) {
  return (log || "")
    .split(/\s+/)
    .filter((t) => t && !/^\d+\.+$/.test(t));
}

// PGN of a finished game. startFen: the position it began from (START_FEN or
// null for the normal start). White and Black are display names.
export function buildPgn({ sans, startFen, result, white, black, date = new Date(), event = "Игра с ботом" }) {
  const custom = startFen && startFen !== START_FEN;
  // a quote or backslash would end the tag value early
  const quote = (s) => String(s).replace(/["\\]/g, "'").replace(/[\r\n]+/g, " ");
  const d = `${date.getFullYear()}.${String(date.getMonth() + 1).padStart(2, "0")}.${String(date.getDate()).padStart(2, "0")}`;
  const head = [
    ["Event", event],
    ["Site", "Dyanis Chess"],
    ["Date", d],
    ["White", white],
    ["Black", black],
    ["Result", result],
  ];
  if (custom) head.push(["SetUp", "1"], ["FEN", startFen]);

  let num = 1;
  let blackFirst = false;
  if (custom) {
    const f = startFen.trim().split(/\s+/);
    blackFirst = f[1] === "b";
    num = Number(f[5]) > 0 ? Number(f[5]) : 1;
  }
  const words = [];
  sans.forEach((san, i) => {
    const whiteMove = blackFirst ? i % 2 === 1 : i % 2 === 0;
    const moveNo = blackFirst ? num + Math.floor((i + 1) / 2) : num + Math.floor(i / 2);
    if (whiteMove) words.push(`${moveNo}.`);
    else if (i === 0) words.push(`${moveNo}...`);
    words.push(san);
  });
  words.push(result);
  return head.map(([k, v]) => `[${k} "${quote(v)}"]`).join("\n") + "\n\n" + words.join(" ") + "\n";
}

const TERMINATION = {
  checkmate: ["Мат", "матом"],
  resign: ["Сдача", "по сдаче соперника"],
  timeout: ["Время вышло", "по времени"],
  stalemate: ["Пат", ""],
  agreement: ["По соглашению", ""],
  repetition: ["Троекратное повторение", ""],
  "fifty-moves": ["Правило 50 ходов", ""],
  "insufficient-material": ["Недостаточно материала", ""],
};

// The same description as describeResult, for a finished online game: result
// is the PGN result, termination what the server says ("checkmate", "resign",
// "timeout", ...), you is the viewer's colour or "" for a spectator (then
// outcome is "neutral" and the title names the winner). reasons: more endings, key -> [reason, how]
// (the chess variants have their own).
export function describeFinished({ result, termination, you, reasons }) {
  const [reason, how] = (reasons && reasons[termination]) || TERMINATION[termination] || [termination || "", ""];
  const winner = result === "1-0" ? "white" : result === "0-1" ? "black" : null;
  if (!winner) {
    return {
      result: "1/2-1/2",
      outcome: you ? "draw" : "neutral",
      reason,
      title: "Ничья",
      subtitle: reason ? `Ничья: ${reason.toLowerCase()}` : "Ничья",
    };
  }
  const outcome = !you ? "neutral" : winner === you ? "win" : "loss";
  const title = !you ? `${SIDE_NAME[winner]} победили` : outcome === "win" ? "Вы победили!" : "Вы проиграли";
  return {
    result: winner === "white" ? "1-0" : "0-1",
    outcome,
    reason,
    title,
    subtitle: you ? `${SIDE_NAME[winner]} победили ${how}`.trim() : reason,
  };
}

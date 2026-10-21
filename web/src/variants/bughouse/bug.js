// Bughouse (Swedish chess): the pure part of its room (covered by `npm test`). Four seats: 0 board 1 White,
// 1 board 1 Black, 2 board 2 White, 3 board 2 Black (the server's variants.BugSeat). Team 0 is seats 0 and 3
// (result "1-0"), team 1 seats 1 and 2 ("0-1"); partners sit on different boards with opposite colours, and
// the piece a player takes goes to the pocket of the partner.
import { BASE_REASONS } from "../describe.js";

export const SEATS = 4;
const OTHER = { white: "black", black: "white" };

export const seatBoard = (seat) => seat >> 1; // 0 or 1
export const seatSide = (seat) => (seat % 2 === 1 ? "black" : "white");
export const seatOf = (board, side) => board * 2 + (side === "black" ? 1 : 0);
export const partnerSeat = (seat) => seatOf(1 - seatBoard(seat), OTHER[seatSide(seat)]);
export const teamOf = (seat) => (seat === 0 || seat === 3 ? 0 : 1);
export const TEAM_SEATS = [
  [0, 3],
  [1, 2],
];

/** "Доска 1, белые" */
export const seatLabel = (seat) => `Доска ${seatBoard(seat) + 1}, ${seatSide(seat) === "white" ? "белые" : "чёрные"}`;

/** The boards in the order they are shown: the viewer's own first, else 1 and 2. */
export const boardOrder = (yourSeat) => (yourSeat >= 0 && seatBoard(yourSeat) === 1 ? [1, 0] : [0, 1]);

/**
 * Which colour stands at the bottom of each board: the viewer's side on their board, the partner's side on the
 * other (partners have opposite colours), White for a spectator or when the person asked for White always.
 */
export function bottomSides(yourSeat, whiteBottom = false) {
  if (whiteBottom || yourSeat < 0) return ["white", "white"];
  const mine = seatBoard(yourSeat);
  const out = ["white", "white"];
  out[mine] = seatSide(yourSeat);
  out[1 - mine] = OTHER[seatSide(yourSeat)];
  return out;
}

/** The names of a team: "Анна и Борис" (a seat nobody has taken is "—"). */
export function teamNames(players, team) {
  return TEAM_SEATS[team].map((s) => players?.[s]?.name || "—").join(" и ");
}

/**
 * The time left of each seat now: the server's `clock` ({ ms: [4], running: [seats], serverNow }) counted down
 * from when it arrived (`receivedAt`, a performance.now() value) to `now` for the seats that run. null without a clock.
 */
export function bugClocks(clock, receivedAt, now) {
  if (!clock) return null;
  const spent = clock.running?.length ? Math.max(0, now - receivedAt) : 0;
  return clock.ms.map((ms, seat) => Math.max(0, ms - (clock.running?.includes(seat) ? spent : 0)));
}

/**
 * The end of a game as the summary card says it: { result, outcome ("win" | "loss" | "neutral"), reason, title,
 * subtitle }. yourSeat < 0 for a spectator (then the title names the winners).
 */
export function describeBugEnd({ result, termination, yourSeat, players, reasons }) {
  const [reason, how] = { ...BASE_REASONS, ...reasons }[termination] || [termination || "", ""];
  const winner = result === "1-0" ? 0 : result === "0-1" ? 1 : null;
  if (winner === null) {
    return { result: "1/2-1/2", outcome: "neutral", reason, title: "Ничья", subtitle: reason ? `Ничья: ${reason.toLowerCase()}` : "Ничья" };
  }
  const names = teamNames(players, winner);
  const outcome = yourSeat < 0 ? "neutral" : teamOf(yourSeat) === winner ? "win" : "loss";
  return {
    result,
    outcome,
    reason,
    title: yourSeat < 0 ? `Победили ${names}` : outcome === "win" ? "Ваша команда победила!" : "Ваша команда проиграла",
    subtitle: yourSeat < 0 ? reason : `Победили ${names} ${how}`.trim(),
  };
}

/**
 * The chat of four for the shared Chat component (online/Chat.jsx): it names the author by `game[m.side]`, so
 * the seats become the sides "s0".."s3". Returns { game, you }.
 */
export function chatForRoom(game, yourSeat) {
  const out = { chat: (game.chat || []).map((m) => ({ side: `s${m.seat}`, text: m.text, at: m.at })) };
  for (let s = 0; s < SEATS; s++) out[`s${s}`] = game.players?.[s] || null;
  return { game: out, you: yourSeat >= 0 ? `s${yourSeat}` : "" };
}

/** The wording of the rematch button: how many of the four have asked. */
export function rematchLabel(rematch, yourSeat) {
  const n = rematch?.offers?.filter(Boolean).length || 0;
  if (rematch?.offers?.[yourSeat]) return `Ждём остальных (${n}/4)`;
  return n > 0 ? `Принять реванш (${n}/4)` : "Реванш";
}

/** The moves of one board for the list: [{ side, items }] rows as in describe.js, from the whole list of moves. */
export const movesOfBoard = (moves, board) => (moves || []).filter((m) => m.board === board);

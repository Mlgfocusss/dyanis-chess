// The logic of the duck screen, apart from React so that `npm test` covers it.

/** The step of the turn the server reports: "move" (a piece) or "duck". */
export const stepOf = (ctx) => ctx.view?.extra?.step || "move";

/** The squares the duck may go to, as board dots: { e5: "dot", ... }. */
export function duckDots(legal) {
  const out = {};
  for (const a of legal || []) if (a[0] === "@") out[a.slice(1)] = "dot";
  return out;
}

/**
 * What a click on a square does. Returns { handled, act?, pending? }: handled false lets the board treat the click
 * as its own (it is not the duck step); act is the action to send; pending the square to remember for a
 * confirmation (with "confirm moves" on, the first click only marks the square).
 */
export function duckClick(square, { myTurn, legal, ui, settings, view }) {
  if (!myTurn || (view?.extra?.step || "move") !== "duck") return { handled: false };
  if (!(legal || []).includes(`@${square}`)) return { handled: true }; // a piece, or the square the duck stands on
  if (settings?.confirmMove && ui?.duckPending !== square) return { handled: true, pending: square };
  return { handled: true, act: `@${square}` };
}

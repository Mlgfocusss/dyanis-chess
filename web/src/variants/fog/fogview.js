// The pure part of the fog of war page (covered by `npm test`).

export const FOG_CLASS = "sq-fog";

// { e5: "sq-fog", ... } for the squares the server says are in the fog (none after the end of the game,
// when it sends no list).
export function fogClasses(view) {
  const out = {};
  const list = view && view.extra && Array.isArray(view.extra.fog) ? view.extra.fog : [];
  for (const sq of list) out[sq] = FOG_CLASS;
  return out;
}

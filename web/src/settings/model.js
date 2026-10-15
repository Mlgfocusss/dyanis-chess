// The settings object: defaults, allowed values and sanitising. Plain
// JavaScript with no imports, so it is covered by `npm test`.
export const DEFAULTS = {
  boardTheme: "slate",
  pieceSet: "default",
  background: "none",
  coordinates: "inside", // "inside" | "outside" | "off"
  highlightLastMove: true,
  moveMethod: "both", // "both" | "drag" | "click"
  animation: "medium", // "none" | "slow" | "medium" | "fast"
  sound: true,
  soundTheme: "default",
};

const CHOICES = {
  coordinates: ["inside", "outside", "off"],
  moveMethod: ["both", "drag", "click"],
  animation: ["none", "slow", "medium", "fast"],
};

// Duration of a piece animation, in milliseconds.
export const ANIMATION_MS = { none: 0, slow: 450, medium: 200, fast: 80 };

// Keeps only known keys of the right type and, for fixed-choice keys, an
// allowed value: a hand-edited or outdated saved object must not break the
// page. Anything else falls back to the default.
export function sanitize(raw) {
  const out = { ...DEFAULTS };
  if (!raw || typeof raw !== "object") return out;
  for (const k of Object.keys(DEFAULTS)) {
    const v = raw[k];
    if (typeof v !== typeof DEFAULTS[k]) continue;
    if (CHOICES[k] && !CHOICES[k].includes(v)) continue;
    out[k] = v;
  }
  return out;
}

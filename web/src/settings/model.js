// The settings object: defaults, allowed values and sanitising. Plain
// JavaScript with no imports, so it is covered by `npm test`.
// Where the settings are kept in this browser (SettingsContext). The script in index.html reads the
// theme from the same place before the page is drawn; a test runs it and checks it still does.
export const SETTINGS_KEY = "dyanis.settings.v1";

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
  playLayout: "side", // "side" | "center": where the board sits on the play page
  focusBoard: false, // only the board and the players are shown on the pages with a board, the board as large as the window allows
  // ---- "Игровой процесс"
  alwaysQueen: false, // a pawn that reaches the last rank becomes a queen at once (Alt asks for the choice)
  confirmGiveUp: true, // ask again before resigning or offering a draw
  showLegalMoves: true, // dots and rings for where the selected piece may go
  whiteBottom: false, // White is at the bottom of the board whichever colour is played
  castling: "standard", // "standard" (the king two squares) | "rook" (the king onto its rook) | "both"
  premoves: false, // a move made in advance in an online game, played when it becomes possible
  timeWarning: true, // the clock turns red and beeps when the time is almost out
  confirmMove: false, // an online move is sent after a press on "confirm"
  multiGames: true, // more than one game at a time
  seekDown: 0, // how much weaker (rating points) an opponent found for me may be; 0 = no limit
  seekUp: 0, // how much stronger; 0 = no limit
  showEval: true, // the engine's evaluation in the review
  autoReview: false, // the review of a finished game starts by itself
  moveIcons: true, // classification badges in the move list of the review
  showCoach: true, // Dionis, the coach, comments on each move of the review
  engineArrows: true, // the engine's three best lines are drawn on the board of the review as arrows
  puzzleLevel: "normal", // "random" | "easier" | "normal" | "harder"
  puzzleRating: true, // the puzzle rating is shown
  puzzlePoints: false, // rated puzzles earn points instead of changing the rating (the server counts them)
  puzzleThemes: false, // the themes of a puzzle are shown before it is solved
  puzzleTimer: false, // the time spent on a puzzle is shown
  // ---- "Интерфейс"
  theme: "dark", // "dark" | "light" | "system" (as the system's own light or dark choice)
  textSize: "normal", // "normal" | "large": the size of the text on the whole site
  reduceMotion: false, // no transitions and animations (on top of the system's own "reduce motion")
  highContrast: false, // stronger borders between the parts of the page
};

// The settings of the "Игровой процесс" page; the others belong to "Доска и фигуры". Each page
// resets only its own.
export const GAMEPLAY_KEYS = [
  "focusBoard", "alwaysQueen", "confirmGiveUp", "showLegalMoves", "whiteBottom", "castling", "premoves", "timeWarning",
  "confirmMove", "multiGames", "seekDown", "seekUp", "showEval", "autoReview", "moveIcons", "showCoach", "engineArrows", "puzzleLevel", "puzzleRating",
  "puzzleThemes", "puzzleTimer", "puzzlePoints",
];

// The settings of the "Интерфейс" page.
export const INTERFACE_KEYS = ["theme", "textSize", "reduceMotion", "highContrast"];

// The defaults of one of the three pages: true is "Игровой процесс", "interface" is "Интерфейс",
// false (or anything else) is "Доска и фигуры", which owns all that the other two do not.
export function defaultsOf(page) {
  const own = (k) => {
    if (INTERFACE_KEYS.includes(k)) return page === "interface";
    if (GAMEPLAY_KEYS.includes(k)) return page === true;
    return page !== true && page !== "interface";
  };
  return Object.fromEntries(Object.entries(DEFAULTS).filter(([k]) => own(k)));
}

// The attributes of <html> the interface settings come down to (a null value = no attribute);
// index.css reacts to them. Pure, so it is covered by the tests.
export function interfaceAttrs(settings) {
  return {
    // dark is the page as it is without the attribute (index.css); the other two change the tokens
    "data-theme": settings.theme === "light" || settings.theme === "system" ? settings.theme : null,
    "data-text-size": settings.textSize === "large" ? "large" : null,
    "data-reduce-motion": settings.reduceMotion ? "on" : null,
    "data-contrast": settings.highContrast ? "high" : null,
  };
}

// Puts the attributes on an element (the root element of the page).
export function applyInterface(el, settings) {
  for (const [name, value] of Object.entries(interfaceAttrs(settings))) {
    if (value === null) el.removeAttribute(name);
    else el.setAttribute(name, value);
  }
}

const CHOICES = {
  coordinates: ["inside", "outside", "off"],
  moveMethod: ["both", "drag", "click"],
  animation: ["none", "slow", "medium", "fast"],
  playLayout: ["side", "center"],
  castling: ["standard", "rook", "both"],
  puzzleLevel: ["random", "easier", "normal", "harder"],
  theme: ["dark", "light", "system"],
  textSize: ["normal", "large"],
};

// Numbers: whole, within the range.
const RANGES = { seekDown: [0, 3000], seekUp: [0, 3000] };

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
    if (RANGES[k]) {
      if (!Number.isFinite(v)) continue;
      out[k] = Math.max(RANGES[k][0], Math.min(RANGES[k][1], Math.round(v)));
      continue;
    }
    out[k] = v;
  }
  return out;
}

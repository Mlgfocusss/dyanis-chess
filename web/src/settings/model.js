// The settings object: defaults, allowed values and sanitising. Plain
// JavaScript with no imports, so it is covered by `npm test`.
// Where the settings are kept in this browser (SettingsContext). The script in index.html reads the
// theme from the same place before the page is drawn; a test runs it and checks it still does.
export const SETTINGS_KEY = "dyanis.settings.v1";

import { detectLanguage } from "../i18n/detect.js";

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
  premoveOn: true, // a move made in advance (online games and puzzles), played when it becomes possible (the key is new: an old saved "off" of premoves must not hold it back)
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
  puzzlePoints: false, // the points are the main number of the rated puzzles (the rating is hidden); the server counts both either way
  puzzleThemes: false, // the themes of a puzzle are shown before it is solved
  puzzleTimer: false, // the time spent on a puzzle is shown
  puzzleAutoNext: true, // after a puzzle is solved the next one opens by itself (the key is new: an old saved "off" of puzzleAuto must not hold it back)
  // correspondence games
  corrConfirmMove: true, // a move in a correspondence game waits for "confirm"
  corrAutoClaim: true, // a win on time is given at once (otherwise the winner claims it); the server reads this one
  corrNextGame: false, // after a move go to the next correspondence game that waits for a move
  corrDays: 3, // days per move of a new correspondence game
  corrNotifySound: true, // a sound when a correspondence game gets a move to answer (notify/)
  corrNotifyBrowser: false, // a system notification for the same, only once the browser has allowed them
  showMoveTime: false, // the time each move took beside the move (the list of moves of a game and of the review)
  // challenges (the server reads all of these but challengeNotify)
  challengesFrom: "friends", // "all" | "friends" | "none": who may challenge me
  challengeMin: 0, // the lowest rating of a challenger, 0 = no limit
  challengeMax: 0, // the highest, 0 = no limit
  challengeFriendsIgnoreRating: true, // friends may challenge whatever their rating
  challengeOffline: true, // challenges are taken while I am away
  challengeNotify: "all", // "all" | "friends" | "none": whose challenges are announced on the page
  // ---- "Интерфейс"
  theme: "dark", // "dark" | "light" | "system" (as the system's own light or dark choice)
  textSize: "normal", // "normal" | "large": the size of the text on the whole site
  reduceMotion: false, // no transitions and animations (on top of the system's own "reduce motion")
  highContrast: false, // stronger borders between the parts of the page
  language: detectLanguage(), // "ru" | "en": the language of the site; the browser's own language until chosen (i18n/)
};

// The settings of the "Игровой процесс" page; the others belong to "Доска и фигуры". Each page
// resets only its own.
export const GAMEPLAY_KEYS = [
  "focusBoard", "alwaysQueen", "confirmGiveUp", "showLegalMoves", "whiteBottom", "castling", "premoveOn", "timeWarning",
  "confirmMove", "multiGames", "seekDown", "seekUp", "showEval", "autoReview", "moveIcons", "showCoach", "engineArrows", "puzzleLevel", "puzzleRating",
  "puzzleThemes", "puzzleTimer", "puzzlePoints", "puzzleAutoNext",
  "corrConfirmMove", "corrAutoClaim", "corrNextGame", "corrDays", "corrNotifySound", "corrNotifyBrowser", "showMoveTime",
  "challengesFrom", "challengeMin", "challengeMax", "challengeFriendsIgnoreRating", "challengeOffline", "challengeNotify",
];

// The settings of the "Интерфейс" page.
export const INTERFACE_KEYS = ["theme", "textSize", "reduceMotion", "highContrast", "language"];

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
  language: ["ru", "en"],
  corrDays: [1, 2, 3, 5, 7, 10, 14],
  challengesFrom: ["all", "friends", "none"],
  challengeNotify: ["all", "friends", "none"],
};

// Numbers: whole, within the range.
const RANGES = { seekDown: [0, 3000], seekUp: [0, 3000], challengeMin: [0, 3500], challengeMax: [0, 3500] };

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

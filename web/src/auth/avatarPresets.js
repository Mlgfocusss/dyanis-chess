// The ready-made avatars and how an avatar value is read. Pure logic (no React), tested.
//
// An account keeps one string for its avatar (the server's users.avatar_url, `avatarUrl` on the wire):
//   ""              the first letter of the name (the default)
//   "preset:<id>"   one of the ready-made avatars below
//   "https://..."   the photo Google sent
// The server accepts exactly these ids (backend/internal/auth/fields.go, avatarPresets); a test keeps the
// two lists equal.

export const PRESET_PREFIX = "preset:";

// Each avatar is a round badge with a gradient of its own hue (as the bots have) and an icon of ui/Icon.jsx.
export const AVATAR_PRESETS = [
  { id: "crown", icon: "crown", label: "Корона", hue: 40 },
  { id: "star", icon: "star", label: "Звезда", hue: 62 },
  { id: "bolt", icon: "bolt", label: "Молния", hue: 215 },
  { id: "flame", icon: "flame", label: "Огонь", hue: 8 },
  { id: "trophy", icon: "trophy", label: "Кубок", hue: 130 },
  { id: "bot", icon: "bot", label: "Бот", hue: 240 },
  { id: "bots", icon: "bots", label: "Боты", hue: 165 },
  { id: "puzzles", icon: "puzzles", label: "Задачи", hue: 190 },
  { id: "board", icon: "board", label: "Доска", hue: 95 },
  { id: "coach", icon: "coach", label: "Тренер", hue: 295 },
  { id: "openings", icon: "openings", label: "Дебюты", hue: 268 },
  { id: "duel", icon: "duel", label: "Дуэль", hue: 335 },
];

const BY_ID = new Map(AVATAR_PRESETS.map((p) => [p.id, p]));

// The gradient of a badge, built as the bots' one is.
export function presetGradient(preset) {
  const h = preset.hue;
  return `linear-gradient(145deg, hsl(${h} 55% 52%), hsl(${(h + 40) % 360} 60% 34%))`;
}

// What an avatar value shows: { kind: "letter" } | { kind: "preset", preset } | { kind: "photo", url }.
// An unknown preset id (an avatar removed later) shows the letter, never a broken picture.
export function parseAvatar(value) {
  const v = typeof value === "string" ? value.trim() : "";
  if (v === "") return { kind: "letter" };
  if (v.startsWith(PRESET_PREFIX)) {
    const preset = BY_ID.get(v.slice(PRESET_PREFIX.length));
    return preset ? { kind: "preset", preset } : { kind: "letter" };
  }
  if (/^https?:\/\//i.test(v)) return { kind: "photo", url: v };
  return { kind: "letter" };
}

export function presetValue(id) {
  return PRESET_PREFIX + id;
}

// The choice the picker shows for a user: "" (letter), "google" (the photo Google sent) or "preset:<id>".
export function avatarChoice(user) {
  const a = parseAvatar(user?.avatarUrl);
  if (a.kind === "preset") return presetValue(a.preset.id);
  if (a.kind === "photo") return "google";
  return "";
}

// The avatar value a choice shows (for the preview before saving).
export function choiceToValue(choice, googleAvatarUrl) {
  if (choice === "google") return googleAvatarUrl || "";
  return parseAvatar(choice).kind === "preset" ? choice : "";
}

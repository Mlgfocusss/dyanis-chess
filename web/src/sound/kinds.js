// Which sound a move makes, from its SAN ("Nf3", "exd5", "O-O", "e8=Q+",
// "Qh4#"). Plain JavaScript with no imports, so it is covered by `npm test`.
export function soundKindForSan(san) {
  if (san.includes("#")) return "end";
  if (san.includes("+")) return "check";
  if (san.startsWith("O-O")) return "castle";
  if (san.includes("=")) return "promote";
  if (san.includes("x")) return "capture";
  return "move";
}

// Sound file names that mean each kind of move, in the naming of the
// common sound packs: lichess ("Move", "Capture", "Check", "Checkmate",
// "Castles", "Victory") and chess.com ("move-self", "move-check",
// "game-end", "game-start"). Compared without case.
const ALIASES = {
  move: ["move", "move-self", "movesel"],
  capture: ["capture", "move-capture"],
  check: ["check", "move-check"],
  castle: ["castle", "castles"],
  promote: ["promote"],
  end: ["checkmate", "game-end", "victory"],
  start: ["game-start"],
};

const AUDIO_ORDER = ["mp3", "m4a", "ogg", "wav"]; // mp3 first: every browser plays it

// entries: [{ name: "Move", ext: "mp3", url }] for the files of one theme
// folder. Returns { move: url, capture: url, ... } for the kinds it has, or
// null when the folder has no plain "move" sound (then it is not a usable
// move-sound theme: instrument packs, notification packs, ...).
export function mapSoundFiles(entries) {
  const byName = {};
  for (const e of entries) {
    const n = e.name.toLowerCase();
    const cur = byName[n];
    if (!cur || AUDIO_ORDER.indexOf(e.ext) < AUDIO_ORDER.indexOf(cur.ext)) byName[n] = e;
  }
  const out = {};
  for (const [kind, names] of Object.entries(ALIASES)) {
    const hit = names.map((n) => byName[n]).find(Boolean);
    if (hit) out[kind] = hit.url;
  }
  return out.move ? out : null;
}

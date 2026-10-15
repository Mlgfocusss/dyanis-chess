// Registry of everything the settings page can offer: board looks, piece
// sets, backgrounds, sound themes, presets.
//
// Nothing here needs editing to add a set. Drop files into the folders
// described in README.md (next to this file) and rebuild: Vite's
// import.meta.glob finds them. Folders that are incomplete are reported
// on the settings page instead of failing silently.

const pieceFiles = import.meta.glob("./pieces/*/*.{svg,png,webp,jpg,jpeg,gif,avif}", {
  eager: true,
  query: "?url",
  import: "default",
});
const boardImages = import.meta.glob("./boards/*.{svg,png,webp,jpg,jpeg,gif,avif}", {
  eager: true,
  query: "?url",
  import: "default",
});
const boardTiles = import.meta.glob("./boards/*/*.{svg,png,webp,jpg,jpeg,gif,avif}", {
  eager: true,
  query: "?url",
  import: "default",
});
const backgroundFiles = import.meta.glob("./backgrounds/*.{svg,png,webp,jpg,jpeg,avif}", {
  eager: true,
  query: "?url",
  import: "default",
});
const soundFiles = import.meta.glob("./sounds/*/*.{mp3,ogg,wav,m4a}", {
  eager: true,
  query: "?url",
  import: "default",
});

// "./pieces/cburnett/wK.svg" -> { folder: "cburnett", name: "wk" };
// depth is the number of folder levels below the registry folder.
function parse(path, depth) {
  const parts = path.replace(/^\.\//, "").split("/"); // ["pieces", "cburnett", "wK.svg"]
  const file = parts[parts.length - 1];
  const name = file.replace(/\.[^.]+$/, "").toLowerCase();
  return { folder: depth === 2 ? parts[1] : null, name };
}

function prettify(name) {
  const s = name.replace(/[-_]+/g, " ").trim();
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function byName(a, b) {
  return a.name.localeCompare(b.name);
}

// ---------------------------------------------------------------- pieces

const PIECE_CODES = ["wp", "wn", "wb", "wr", "wq", "wk", "bp", "bn", "bb", "br", "bq", "bk"];

const rawSets = {};
for (const [path, url] of Object.entries(pieceFiles)) {
  const { folder, name } = parse(path, 2);
  (rawSets[folder] ||= {})[name] = url;
}

export const DEFAULT_PIECES = { id: "default", name: "Стандартные", builtin: true };

export const pieceSets = [DEFAULT_PIECES];
// Folders that exist but lack some of the 12 pieces: shown as a warning.
export const incompletePieceSets = [];
for (const folder of Object.keys(rawSets).sort()) {
  const files = rawSets[folder];
  const missing = PIECE_CODES.filter((c) => !files[c]);
  if (missing.length) {
    incompletePieceSets.push({ id: folder, name: prettify(folder), missing });
  } else {
    pieceSets.push({
      id: folder,
      name: prettify(folder),
      // react-chessboard piece keys are wP, bN, ...
      files: Object.fromEntries(PIECE_CODES.map((c) => [c[0] + c[1].toUpperCase(), files[c]])),
    });
  }
}

// ---------------------------------------------------------------- boards

// Plain colours: light and dark are the two square colours.
export const colorBoards = [
  { id: "slate", name: "Сланец", light: "#dfe6ee", dark: "#7a93ad" },
  { id: "green", name: "Зелёная", light: "#eeeed2", dark: "#769656" },
  { id: "brown", name: "Дерево", light: "#f0d9b5", dark: "#b58863" },
  { id: "blue", name: "Синяя", light: "#dee3e6", dark: "#8ca2ad" },
  { id: "ice", name: "Лёд", light: "#e8f1f8", dark: "#7fa6c9" },
  { id: "purple", name: "Фиолетовая", light: "#efe9f7", dark: "#8877b7" },
  { id: "coral", name: "Коралл", light: "#f6e7d7", dark: "#c97b63" },
  { id: "sand", name: "Песок", light: "#efe6cf", dark: "#c2a36a" },
  { id: "night", name: "Ночь", light: "#9aa5b5", dark: "#3b4559" },
  { id: "mono", name: "Графит", light: "#e5e5e5", dark: "#7d7d7d" },
].map((b) => ({ ...b, kind: "color" }));

// A whole-board picture: boards/<name>.png
const imageBoards = Object.entries(boardImages)
  .map(([path, url]) => {
    const { name } = parse(path, 1);
    return { id: `img:${name}`, name: prettify(name), kind: "image", image: url };
  })
  .sort(byName);

// Two square textures: boards/<name>/light.png and dark.png
const tileFolders = {};
for (const [path, url] of Object.entries(boardTiles)) {
  const { folder, name } = parse(path, 2);
  (tileFolders[folder] ||= {})[name] = url;
}
export const incompleteBoards = [];
const tileBoards = [];
for (const folder of Object.keys(tileFolders).sort()) {
  const f = tileFolders[folder];
  if (f.light && f.dark) {
    tileBoards.push({ id: `tiles:${folder}`, name: prettify(folder), kind: "tiles", light: f.light, dark: f.dark });
  } else {
    incompleteBoards.push({ id: folder, name: prettify(folder), missing: ["light", "dark"].filter((k) => !f[k]) });
  }
}

export const boardThemes = [...colorBoards, ...imageBoards, ...tileBoards];

// ----------------------------------------------------------- backgrounds

export const backgrounds = [
  { id: "none", name: "Стандартный", kind: "builtin", css: "none", swatch: "var(--bg)" },
  { id: "midnight", name: "Полночь", kind: "builtin", css: "linear-gradient(160deg, #0b1220 0%, #16223d 100%)" },
  { id: "forest", name: "Лес", kind: "builtin", css: "linear-gradient(160deg, #0a1512 0%, #14332a 100%)" },
  { id: "ocean", name: "Океан", kind: "builtin", css: "linear-gradient(160deg, #08141f 0%, #0f3350 100%)" },
  { id: "dusk", name: "Сумерки", kind: "builtin", css: "linear-gradient(160deg, #150f1f 0%, #2d1d45 100%)" },
  { id: "graphite", name: "Графит", kind: "builtin", css: "linear-gradient(160deg, #0f1114 0%, #23272e 100%)" },
  ...Object.entries(backgroundFiles)
    .map(([path, url]) => {
      const { name } = parse(path, 1);
      return { id: `img:${name}`, name: prettify(name), kind: "image", image: url };
    })
    .sort(byName),
];

// CSS value of the page background. Photos get a dark veil so the text on
// top stays readable.
export function backgroundCss(bg) {
  if (!bg || bg.id === "none") return "none";
  if (bg.kind === "image") return `linear-gradient(rgba(11,18,32,0.82), rgba(11,18,32,0.82)), url("${bg.image}")`;
  return bg.css;
}

// ---------------------------------------------------------------- sounds

export const SOUND_KINDS = ["move", "capture", "check", "castle", "promote", "end", "start"];

export const DEFAULT_SOUND = { id: "default", name: "Стандартная (синтез)", builtin: true };

const soundFolders = {};
for (const [path, url] of Object.entries(soundFiles)) {
  const { folder, name } = parse(path, 2);
  (soundFolders[folder] ||= {})[name] = url;
}
export const soundThemes = [
  DEFAULT_SOUND,
  ...Object.keys(soundFolders)
    .sort()
    .map((folder) => ({ id: folder, name: prettify(folder), files: soundFolders[folder] })),
];

// --------------------------------------------------------------- presets

// A preset is a bundle of looks. Presets name only things that always
// exist; the piece set stays the user's own choice unless a preset sets it.
export const presets = [
  { id: "classic", name: "Классика", boardTheme: "brown", background: "none" },
  { id: "slate", name: "Сланец", boardTheme: "slate", background: "none" },
  { id: "forest", name: "Лес", boardTheme: "green", background: "forest" },
  { id: "night", name: "Ночная партия", boardTheme: "night", background: "midnight" },
  { id: "ice", name: "Лёд", boardTheme: "ice", background: "ocean" },
  { id: "dusk", name: "Сумерки", boardTheme: "purple", background: "dusk" },
];

// --------------------------------------------------------------- lookups
// A saved id may name a folder that has since been removed: fall back.

export const getBoardTheme = (id) => boardThemes.find((b) => b.id === id) || boardThemes[0];
export const getPieceSet = (id) => pieceSets.find((s) => s.id === id) || DEFAULT_PIECES;
export const getBackground = (id) => backgrounds.find((b) => b.id === id) || backgrounds[0];
export const getSoundTheme = (id) => soundThemes.find((s) => s.id === id) || DEFAULT_SOUND;

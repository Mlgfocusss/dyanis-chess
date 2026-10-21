// Every folder web/src/variants/<id>/ with an index.js (or .jsx) is a variant: Vite's import.meta.glob finds
// them, nothing else needs editing to add one (the menu, the catalogue and the lobby are built from this list).
// Problems with a definition are listed in PROBLEMS and shown on the catalogue page, not thrown.
import { buildRegistry } from "./registry-core.js";

const modules = import.meta.glob("./*/index.{js,jsx}", { eager: true });
const built = buildRegistry(Object.entries(modules).map(([path, mod]) => ({ path, def: mod.default })));

/** Every variant, hidden ones included (a game of "classic" is played through its link). */
export const ALL_VARIANTS = built.variants;
/** The variants the menu and the catalogue offer. */
export const VARIANTS = built.variants.filter((v) => !v.hidden);
export const PROBLEMS = built.problems;
export const getVariant = (id) => ALL_VARIANTS.find((v) => v.id === id) || null;

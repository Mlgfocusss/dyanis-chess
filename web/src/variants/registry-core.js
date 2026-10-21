// The list of chess variants of the site, built from the definitions in web/src/variants/<id>/index.js(x).
// Pure (the file search itself is in registry.js), so `npm test` covers it.
//
// A definition is the default export of the folder's index file:
//   id        the folder's name: letters, digits and "-", the same as the server's variant id
//   name      "Атомные шахматы"
//   short     one sentence for the card
//   rules     ["...", "..."] the rules in a few short lines (the card shows the first three, the lobby all)
//   icon      a name from ui/Icon.jsx ("variants" by default)
//   hidden    true: not in the menu and the catalogue (the "classic" fixture)
//   order     where it stands among the others (smaller first, 100 by default, then by name)
// and, all optional, what makes the variant different on the screen (see VARIANTS-PLAN.md, 5.F.3):
//   reasons, lobby, pieces, captured, boardProps, onSquare, PanelTop, PanelBottom, Side, Controls, hint, moveText.

const ID = /^[a-z0-9][a-z0-9-]*$/;

// The folder name of a glob path: "./atomic/index.js" -> "atomic".
export function folderOf(path) {
  const m = /^\.\/([^/]+)\/index\.[jt]sx?$/.exec(path);
  return m ? m[1] : "";
}

// Checks one definition. Returns { variant, problems }: the definition with its defaults filled in
// (null when it cannot be used) and what is wrong with it, in words for the developer.
export function normalizeVariant(def, folder) {
  const problems = [];
  const where = `variants/${folder}`;
  if (!def || typeof def !== "object") return { variant: null, problems: [`${where}: index.js ничего не экспортирует по умолчанию`] };
  if (def.id !== folder) problems.push(`${where}: id «${def.id}» не совпадает с именем папки`);
  else if (!ID.test(def.id)) problems.push(`${where}: id должен состоять из латинских букв, цифр и «-»`);
  if (typeof def.name !== "string" || !def.name.trim()) problems.push(`${where}: нет name`);
  if (typeof def.short !== "string" || !def.short.trim()) problems.push(`${where}: нет short`);
  if (def.rules !== undefined && !(Array.isArray(def.rules) && def.rules.every((r) => typeof r === "string"))) problems.push(`${where}: rules должен быть списком строк`);
  if (problems.length) return { variant: null, problems };
  return {
    variant: { icon: "variants", hidden: false, order: 100, rules: [], ...def },
    problems,
  };
}

// entries: [{ path, def }] from the file search. Returns { variants (sorted), problems }.
export function buildRegistry(entries) {
  const variants = [];
  const problems = [];
  const seen = new Set();
  for (const { path, def } of entries) {
    const folder = folderOf(path);
    if (!folder) {
      problems.push(`${path}: ожидалось variants/<id>/index.js`);
      continue;
    }
    const r = normalizeVariant(def, folder);
    problems.push(...r.problems);
    if (!r.variant) continue;
    if (seen.has(r.variant.id)) {
      problems.push(`variants/${folder}: id «${r.variant.id}» уже занят`);
      continue;
    }
    seen.add(r.variant.id);
    variants.push(r.variant);
  }
  variants.sort((a, b) => a.order - b.order || a.name.localeCompare(b.name, "ru"));
  return { variants, problems };
}

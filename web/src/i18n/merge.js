// Joins the dictionaries of the areas; the same key in two areas is a mistake.
export function merge(name, ...parts) {
  const out = {};
  for (const part of parts) {
    for (const [k, v] of Object.entries(part)) {
      if (k in out) throw new Error(`i18n: duplicate key "${k}" in the ${name} dictionary`);
      out[k] = v;
    }
  }
  return out;
}

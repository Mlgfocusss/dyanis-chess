// Opening names are shown in Russian; the English name stays in the tooltip.
// The server adds `nameRu` to every opening it sends (and fills it in for
// reports saved before the translation existed); `name` stays English.

// The name to show: Russian when there is one, otherwise the English one.
export function openingName(o) {
  return o ? o.nameRu || o.name || "" : "";
}

// The tooltip: the English name, only when it differs from what is shown.
export function openingTitle(o) {
  if (!o || !o.name) return undefined;
  return o.nameRu && o.nameRu !== o.name ? o.name : undefined;
}

// Same for a summary row of "my games": { opening, openingRu }.
export function summaryOpening(summary) {
  if (!summary || !summary.opening) return { text: "", title: undefined };
  const ru = summary.openingRu;
  return { text: ru || summary.opening, title: ru && ru !== summary.opening ? summary.opening : undefined };
}

// Opening names follow the language of the site: in Russian the Russian name is shown and the English one
// stays in the tooltip; in English the English name is shown and there is no tooltip.
// The server adds `nameRu` to every opening it sends (and fills it in for reports saved before the
// translation existed); `name` stays English. The functions read the current language (getLanguage), so
// the component that shows a name has to be redrawn on a change of language (it calls useT()).
import { getLanguage } from "../i18n/index.js";

// The name to show: in Russian the Russian one when there is one, otherwise the English one.
export function openingName(o) {
  if (!o) return "";
  return getLanguage() === "en" ? o.name || o.nameRu || "" : o.nameRu || o.name || "";
}

// The tooltip: the English name, only in Russian and only when it differs from what is shown.
export function openingTitle(o) {
  if (getLanguage() === "en") return undefined;
  if (!o || !o.name) return undefined;
  return o.nameRu && o.nameRu !== o.name ? o.name : undefined;
}

// Same for a summary row of "my games": { opening, openingRu }.
export function summaryOpening(summary) {
  if (!summary || !summary.opening) return { text: "", title: undefined };
  if (getLanguage() === "en") return { text: summary.opening, title: undefined };
  const ru = summary.openingRu;
  return { text: ru || summary.opening, title: ru && ru !== summary.opening ? summary.opening : undefined };
}

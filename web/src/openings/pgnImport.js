import { tr } from "../i18n/index.js";
// Pure helpers of the PGN import, with no imports, so `npm test` covers them.

// "1 ход", "2 хода", "5 ходов"
export function plural(n, one, few, many) {
  const m100 = n % 100;
  const m10 = n % 10;
  if (m100 >= 11 && m100 <= 14) return `${n} ${many}`;
  if (m10 === 1) return `${n} ${one}`;
  if (m10 >= 2 && m10 <= 4) return `${n} ${few}`;
  return `${n} ${many}`;
}

// What an import did, for the person: { games, skipped, added, moves } from the server.
export function importSummary(st) {
  const games = tr("openings.imp.games", { n: st.games });
  let text =
    st.added === 0
      ? tr("openings.imp.none", { games })
      : tr("openings.imp.added", { moves: tr("openings.imp.moves", { n: st.added }), games });
  if (st.transposed > 0) {
    text += ` ${tr("openings2.trans.importN", { n: st.transposed })}.`;
  }
  if (st.skipped > 0) {
    text += tr("openings.imp.skipTail", { skipped: tr("openings.imp.skippedN", { n: st.skipped }) });
  }
  return text;
}

// The name a file suggests for the repository: its own name without the extension.
export function nameFromFile(fileName) {
  return (fileName || "").replace(/\.[^.\\/]+$/, "").replace(/[_]+/g, " ").trim().slice(0, 80);
}

import { useT } from "../i18n/index.js";
import "./puzzles.css";

// The tabs of the puzzle section (short names: five of them share one row; the menu has the full ones). They change the address, and the page follows it (the menu on the
// left does the same).
export const MODES = [
  { id: "rated", label: "pz.tabRated", path: "#puzzles" },
  { id: "themes", label: "pz.tabThemes", path: "#puzzles/themes" },
  { id: "daily", label: "pz.tabDay", path: "#puzzles/daily" },
  { id: "rush", label: "pz.tabRush", path: "#puzzles/rush" },
  { id: "duel", label: "pz.tabDuel", path: "#puzzles/duel" },
];

export function chooseMode(id) {
  window.location.hash = MODES.find((m) => m.id === id).path;
}

export default function PuzzleTabs({ mode }) {
  const t = useT();
  return (
    <div className="tabs puzzles__tabs" role="tablist" aria-label={t("pzTabs.6")}>
      {MODES.map((m) => (
        <button
          key={m.id}
          type="button"
          role="tab"
          aria-selected={mode === m.id}
          className={`tabs__tab${mode === m.id ? " tabs__tab--active" : ""}`}
          onClick={() => chooseMode(m.id)}
        >
          {t(m.label)}
        </button>
      ))}
    </div>
  );
}

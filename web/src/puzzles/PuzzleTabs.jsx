// The tabs of the puzzle section (short names: five of them share one row; the menu has the full ones). They change the address, and the page follows it (the menu on the
// left does the same).
export const MODES = [
  { id: "rated", label: "Рейтинг", path: "#puzzles" },
  { id: "themes", label: "Темы", path: "#puzzles/themes" },
  { id: "daily", label: "День", path: "#puzzles/daily" },
  { id: "rush", label: "Штурм", path: "#puzzles/rush" },
  { id: "duel", label: "Дуэль", path: "#puzzles/duel" },
];

export function chooseMode(id) {
  window.location.hash = MODES.find((m) => m.id === id).path;
}

export default function PuzzleTabs({ mode }) {
  return (
    <div className="tabs puzzles__tabs" role="tablist" aria-label="Разделы задач">
      {MODES.map((m) => (
        <button
          key={m.id}
          type="button"
          role="tab"
          aria-selected={mode === m.id}
          className={`tabs__tab${mode === m.id ? " tabs__tab--active" : ""}`}
          onClick={() => chooseMode(m.id)}
        >
          {m.label}
        </button>
      ))}
    </div>
  );
}

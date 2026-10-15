// Move classes of internal/analysis: label, badge colour and the ink that
// stays readable on it. icon names a glyph from ui/Icon.jsx; otherwise
// text is drawn.
export const CLASSES = {
  book: { label: "Теория", text: "", icon: "openings", color: "#64748b", ink: "#ffffff" },
  best: { label: "Лучший ход", text: "", icon: "star", color: "#22c55e", ink: "#052e16" },
  excellent: { label: "Отличный ход", text: "!", color: "#84cc16", ink: "#1a2e05" },
  good: { label: "Хороший ход", text: "✓", color: "#38bdf8", ink: "#082f49" },
  inaccuracy: { label: "Неточность", text: "?!", color: "#facc15", ink: "#422006" },
  mistake: { label: "Ошибка", text: "?", color: "#fb923c", ink: "#431407" },
  blunder: { label: "Зевок", text: "??", color: "#ef4444", ink: "#ffffff" },
};

// Order used in the summary table.
export const SUMMARY_ORDER = ["book", "best", "excellent", "good", "inaccuracy", "mistake", "blunder"];

// Evaluation as the reader expects it: "+0.35", "-2.10", "M3" / "-M2".
// score is centipawns from White's point of view; mate is moves to a
// forced mate (positive: White mates), 0 when none.
export function formatEval(ev) {
  if (!ev) return "";
  if (ev.terminal === "checkmate") return "#";
  if (ev.mate) return `${ev.mate < 0 ? "-" : ""}M${Math.abs(ev.mate)}`;
  const pawns = ev.score / 100;
  return `${pawns > 0 ? "+" : ""}${pawns.toFixed(2)}`;
}

// "e2e4" -> { from: "e2", to: "e4" }
export function uciSquares(uci) {
  if (!uci || uci.length < 4) return null;
  return { from: uci.slice(0, 2), to: uci.slice(2, 4) };
}

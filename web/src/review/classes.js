// Move classes of internal/analysis: label, badge colour and the ink that
// stays readable on it, and a sentence for the "now playing" card. icon names
// a glyph from ui/Icon.jsx; otherwise text is drawn. From the best to the
// worst: brilliant, great, best, excellent, good, then theory (not judged),
// inaccuracy, mistake, blunder.
import { tOr } from "../i18n/index.js";

export const CLASSES = {
  brilliant: {
    label: "Блестящий ход",
    text: "!!",
    color: "#1baca6",
    ink: "#ffffff",
    hint: "Жертва материала, которая оправдана: ход ничего не потерял, а позиция осталась хорошей.",
  },
  great: {
    label: "Сильный ход",
    text: "!",
    color: "#5b8bb3",
    ink: "#ffffff",
    hint: "Единственный хороший ход: другие заметно хуже.",
  },
  best: { label: "Лучший ход", text: "", icon: "star", color: "#22c55e", ink: "#052e16", hint: "Это ход, который выбрал бы движок." },
  excellent: { label: "Отличный ход", text: "", icon: "thumb", color: "#84cc16", ink: "#1a2e05", hint: "Почти не хуже лучшего." },
  good: { label: "Хороший ход", text: "✓", color: "#38bdf8", ink: "#082f49", hint: "Нормальный ход, но был чуть лучше." },
  book: { label: "Теория", text: "", icon: "openings", color: "#64748b", ink: "#ffffff", hint: "Ход из дебютной теории." },
  miss: {
    label: "Упущенная возможность",
    text: "×",
    color: "#f59e0b",
    ink: "#451a03",
    hint: "Соперник ошибся, а вы не воспользовались этим: преимущество упущено.",
  },
  inaccuracy: { label: "Неточность", text: "?!", color: "#facc15", ink: "#422006", hint: "Небольшая потеря: шансы заметно просели." },
  mistake: { label: "Ошибка", text: "?", color: "#fb923c", ink: "#431407", hint: "Серьёзная потеря: позиция стала заметно хуже." },
  blunder: { label: "Зевок", text: "??", color: "#ef4444", ink: "#ffffff", hint: "Грубая ошибка, которая резко меняет оценку." },
};

// The name and the hint of a class in the language of the site (t: from useT).
export const classLabel = (t, cls) => tOr(t, `review.class.${cls}.label`, CLASSES[cls]?.label || "");
export const classHint = (t, cls) => tOr(t, `review.class.${cls}.hint`, CLASSES[cls]?.hint || "");

// Order used in the summary table (best first, theory with the good ones).
export const SUMMARY_ORDER = ["brilliant", "great", "best", "excellent", "good", "book", "miss", "inaccuracy", "mistake", "blunder"];

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

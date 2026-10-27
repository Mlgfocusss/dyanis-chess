// Chess960 (Fischer random chess). The server chooses the setup (a number 0..959; the lobby may name one)
// and both players get the same; the number is shown in the room card. The castling is the board's job:
// boardrules.js knows the Chess960 form (the king onto its own rook, or onto the square it will stand on).
// Rules and the numbering: backend/internal/variants/chess960.go.
import { useState } from "react";
import Icon from "../../ui/Icon.jsx";
import { CLASSIC, COUNT, describeSetup, parseNumber, rankLetters } from "./setup.js";
import { useT } from "../../i18n/index.js";
import "./chess960.css";

// The lobby form: a random setup (the default) or the number of one.
function SetupOptions({ value, onChange }) {
  const t = useT();
  const [named, setNamed] = useState(value?.position !== undefined);
  const [text, setText] = useState(value?.position !== undefined ? String(value.position) : "");
  const n = parseNumber(text);
  const bad = named && (n === null || Number.isNaN(n));

  function pick(number) {
    setNamed(number);
    if (!number) return onChange({});
    const v = n === null || Number.isNaN(n) ? CLASSIC : n;
    setText(String(v));
    onChange({ position: v });
  }
  function type(v) {
    setText(v);
    const m = parseNumber(v);
    // a text that is no number goes on as it is: the server refuses it in words
    onChange({ position: m === null || Number.isNaN(m) ? v : m });
  }

  return (
    <div className="c960-opts">
      <div className="c960-opts__row">
        <button type="button" className={`chip${!named ? " chip--on" : ""}`} aria-pressed={!named} onClick={() => pick(false)}>
          {t("vc960.1")}
        </button>
        <button type="button" className={`chip${named ? " chip--on" : ""}`} aria-pressed={named} onClick={() => pick(true)}>
          {t("vc960.2")}
        </button>
        {named && (
          <label className="c960-opts__num">
            <span className="c960-opts__lab">{t("vc960.3")}{COUNT - 1})</span>
            <input
              className="field c960-opts__input"
              inputMode="numeric"
              value={text}
              onChange={(e) => type(e.target.value)}
              aria-invalid={bad}
              aria-label={t("vc960.4", { p: COUNT - 1 })}
            />
          </label>
        )}
        {named && (
          <button type="button" className="btn btn--ghost" onClick={() => type(String(CLASSIC))}>
            {CLASSIC}{t("vc960.5")}
          </button>
        )}
      </div>
      <p className={`c960-opts__note${bad ? " c960-opts__note--bad" : ""}`}>
        {bad
          ? t("vc960.6", { p: COUNT - 1 })
          : named
            ? t("vc960.7")
            : t("vc960.8")}
      </p>
    </div>
  );
}

function SetupCard({ view, game }) {
  const t = useT();
  const x = view?.extra;
  if (!x || typeof x.number !== "number") return null;
  return (
    <section className="card c960-card">
      <h2>
        <Icon name="chess960" size={18} /> {describeSetup(x.number)}
      </h2>
      <div className="c960-rank" role="img" aria-label={t("vc960.9", { p: rankLetters(x.rank).join(" ") })}>
        {rankLetters(x.rank).map((l, i) => (
          <span key={i} className="c960-rank__cell">
            {l}
          </span>
        ))}
      </div>
      <p>{t("vc960.10")}</p>
      <p>{t("vc960.11")}</p>
      {game.status === "ended" && <p className="c960-card__over">{t("vc960.12")} {x.number}.</p>}
    </section>
  );
}

export default {
  id: "chess960",
  name: "Chess960",
  short: "Фигуры первой горизонтали стоят в случайном порядке (одна из 960 расстановок), рокировка по правилам Фишера.",
  rules: [
    "Пешки стоят как обычно, а фигуры первой горизонтали расставлены случайно: слоны на полях разного цвета, король между ладьями. Всего 960 расстановок, у каждой свой номер; 518 — обычная.",
    "Расстановку выбирает сервер, обоим игрокам одну и ту же (в лобби можно назвать номер). Номер виден в партии.",
    "Рокировка по правилам Фишера: король встаёт на g (короткая) или c (длинная), ладья на f или d, где бы они ни стояли. Все клетки между ними и их конечными полями свободны, король не под боем и не проходит через битые поля.",
    "Рокировка делается так: нажмите короля и затем свою ладью (или клетку, куда король встанет) либо перетащите короля на ладью. Способ можно выбрать в настройках.",
  ],
  icon: "chess960",
  order: 20,
  // After the game: the review by the engine (backend/internal/analysis/variant.go). No opening names, no theory moves,
  // and the engine never suggests a castling while one is still possible: it searches positions without castling rights.
  review: true,
  reviewHint: "Разбор движком, как у обычных партий, но без дебютной теории: у Chess960 нет названий дебютов. Рокировка в оценке есть, но лучшим ходом движок её не назовёт.",
  lobby: { defaults: {}, Options: SetupOptions },
  Side: SetupCard,
};

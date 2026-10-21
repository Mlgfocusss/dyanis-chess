import { useEffect, useMemo, useState } from "react";
import { formatEval } from "./classes.js";
import { numberedSans } from "./branch.js";
import Icon from "../ui/Icon.jsx";

// The engine's best lines of the position on the board (MultiPV) as slim rows, three of them taking
// about as much room as one: the evaluation and the moves on one line, cut off where the panel ends
// (with the whole line in the tooltip). A row opens on a press of its chevron or of the evaluation
// and then shows the whole line; each move can be pressed to play the line up to it on the board.
// lines: [{score, mate, pv}] or null while they are being worked out; first is shown meanwhile
// ({score, mate, pv}, the one the review already has). onPlay(pv, k) plays pv[0..k].
export default function EngineLines({ fen, lines, first, showEval, onPlay }) {
  const rows = useMemo(() => (lines && lines.length ? lines : first?.pv?.length ? [first] : []), [lines, first]);
  const numbered = useMemo(() => rows.map((r) => numberedSans(fen, r.pv.map((san) => ({ san })))), [fen, rows]);
  const [open, setOpen] = useState(-1); // the row shown whole
  useEffect(() => setOpen(-1), [fen]);
  if (rows.length === 0) return null;
  const loading = !lines;
  return (
    <section className="linebox" aria-label="Лучшие ходы движка">
    <h4 className="linebox__title">Лучшие ходы движка <span>= стрелки на доске</span></h4>
    <ol className="lines">
      {rows.map((r, i) => {
        const whole = open === i;
        return (
          <li key={i} className={`lines__row${whole ? " lines__row--open" : ""}`}>
            <span className={`lines__rank lines__rank--${i + 1}`} title={`Линия ${i + 1}: стрелка на доске`}>
              {i + 1}
            </span>
            <button
              type="button"
              className={`lines__eval lines__eval--${(r.mate ? r.mate > 0 : r.score >= 0) ? "w" : "b"}`}
              onClick={() => setOpen(whole ? -1 : i)}
              aria-expanded={whole}
              title={whole ? "Свернуть линию" : "Показать линию целиком"}
            >
              {showEval ? formatEval(r) : i + 1}
            </button>
            <span className="lines__moves" title={numbered[i].map((m) => (m.label ? `${m.label} ${m.san}` : m.san)).join(" ")}>
              {numbered[i].map((m, k) => (
                <button key={k} type="button" className="pv-move" onClick={() => onPlay(r.pv, k)} title="Сыграть эту линию до этого хода на доске">
                  {m.label && <span className="pv-move__no">{m.label}</span>}
                  {m.san}
                </button>
              ))}
            </span>
            <button type="button" className="lines__chev" onClick={() => setOpen(whole ? -1 : i)} aria-label={whole ? "Свернуть линию" : "Показать линию целиком"} tabIndex={-1}>
              <Icon name="chevron" size={14} />
            </button>
          </li>
        );
      })}
      {loading && <li className="lines__more review__thinking">ищу ещё две линии…</li>}
    </ol>
    </section>
  );
}

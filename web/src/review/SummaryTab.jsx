import ClassBadge from "./ClassBadge.jsx";
import EvalChart from "./EvalChart.jsx";
import { CLASSES, SUMMARY_ORDER } from "./classes.js";
import { chartMarks, momentPlies, moveLabel, summaryRows } from "./moments.js";

// The "Итог" tab of the review panel: the accuracy of both players, how many moves of each class each
// made (the table of chess.com), the moves that turned the game and the whole curve. Pressing a move
// opens it on the board.
export default function SummaryTab({ report, game, whiteName, blackName, ply, onGo, onOpen, showEval, labelOf }) {
  const { moves, evals } = report;
  const rows = summaryRows(report.counts, SUMMARY_ORDER);
  const keyPlies = (report.key || []).filter((p) => moves[p - 1]);
  const finds = momentPlies(moves, { kinds: ["brilliant", "great"] });
  return (
    <div className="sumtab">
      <div className="accuracy">
        <div className="accuracy__box">
          <span className="accuracy__name">{whiteName}</span>
          <span className="accuracy__num">{report.accuracy[0].toFixed(1)}</span>
          <span className="accuracy__cap">точность</span>
        </div>
        <div className="accuracy__box accuracy__box--black">
          <span className="accuracy__name">{blackName}</span>
          <span className="accuracy__num">{report.accuracy[1].toFixed(1)}</span>
          <span className="accuracy__cap">точность</span>
        </div>
      </div>

      {showEval && evals.length > 1 && (
        <section className="sumtab__block">
          <h3>График оценки</h3>
          <EvalChart evals={evals} ply={ply} marks={chartMarks(moves)} onSelect={onGo} labelOf={labelOf} />
        </section>
      )}

      {rows.length > 0 && (
        <section className="sumtab__block">
          <h3>Ходы по классам</h3>
          <table className="counts">
            <thead>
              <tr>
                <th className="counts__n">{whiteName}</th>
                <th />
                <th className="counts__n">{blackName}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.cls}>
                  <td className="counts__n">{r.white}</td>
                  <td>
                    <ClassBadge cls={r.cls} />
                    {CLASSES[r.cls].label}
                  </td>
                  <td className="counts__n">{r.black}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {keyPlies.length > 0 && (
        <section className="sumtab__block">
          <h3>Ключевые моменты</h3>
          <div className="moments">
            {keyPlies.map((p) => (
              <MomentButton key={p} move={moves[p - 1]} active={p === ply} onGo={onOpen} loss />
            ))}
          </div>
        </section>
      )}

      {finds.length > 0 && (
        <section className="sumtab__block">
          <h3>Лучшие моменты</h3>
          <div className="moments">
            {finds.map((p) => (
              <MomentButton key={p} move={moves[p - 1]} active={p === ply} onGo={onOpen} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function MomentButton({ move, active, onGo, loss = false }) {
  return (
    <button type="button" className={`moment${active ? " moment--on" : ""}`} onClick={() => onGo(move.ply)} title={CLASSES[move.class]?.label}>
      <ClassBadge cls={move.class} />
      <span className="moment__san">{moveLabel(move)}</span>
      <span className="moment__who">{move.color === "black" ? "чёрные" : "белые"}</span>
      {loss && move.loss > 0 && <small className="moment__loss">−{move.loss.toFixed(0)}%</small>}
    </button>
  );
}

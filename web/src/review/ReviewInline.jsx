import { useEffect } from "react";
import ThemedBoard from "../components/ThemedBoard.jsx";
import Icon from "../ui/Icon.jsx";
import ReviewView from "./ReviewView.jsx";
import { useAnalysis } from "./useAnalysis.js";
import "./review.css";

// The review of a game that was just played, shown in place of the game
// screen: the board stays where it was and the side panel turns into the
// analysis, so there is no jump to another page. It starts the analysis
// itself and shows its progress next to the final position.
//   pgn          the game
//   finalFen     the position to show while the engine works
//   orientation  the side the viewer played, so the board does not turn over
//   onClose      back to the game
export default function ReviewInline({ pgn, finalFen, orientation = "white", depth = 14, onClose }) {
  const { job, error, busy, start } = useAnalysis();

  useEffect(() => {
    start({ pgn, depth });
  }, [pgn]); // eslint-disable-line react-hooks/exhaustive-deps

  if (job?.status === "done" && job.report) {
    return <ReviewView key={job.id} job={job} inline orientation={orientation} onBack={onClose} />;
  }

  const pct = job && job.total ? Math.round((100 * job.done) / job.total) : 0;
  return (
    <div className="review review--inline">
      <div className="review__main">
        <div className="review__boardrow">
          <div className="review__board review__board--waiting">
            <ThemedBoard id="ReviewWaitingBoard" position={finalFen} orientation={orientation} arePiecesDraggable={false} />
          </div>
        </div>
      </div>
      <aside className="review__side">
        <div className="card review__wait">
          <h2>{error ? "Разбор не удался" : "Анализирую партию…"}</h2>
          {error ? (
            <div className="banner banner--error">{error}</div>
          ) : (
            <div className="progress" aria-live="polite">
              <div className="progress__bar" style={{ width: `${busy ? Math.max(pct, 3) : 100}%` }} />
              <span>
                {!job || job.status === "queued" ? "В очереди…" : `Оценено позиций: ${job.done} из ${job.total}`}
              </span>
            </div>
          )}
          <p className="review__hint">Движок оценивает каждый ход и подбирает значки: от зевка до блестящего хода.</p>
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            <Icon name="back" size={16} /> К партии
          </button>
        </div>
      </aside>
    </div>
  );
}

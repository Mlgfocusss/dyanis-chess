import { useEffect } from "react";
import ThemedBoard from "../components/ThemedBoard.jsx";
import Icon from "../ui/Icon.jsx";
import ReviewView from "./ReviewView.jsx";
import { useAnalysis } from "./useAnalysis.js";
import { useT } from "../i18n/index.js";
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
  const t = useT();
  const { job, error, busy, start } = useAnalysis();

  useEffect(() => {
    start({ pgn, depth });
  }, [pgn]); // eslint-disable-line react-hooks/exhaustive-deps

  // A finished review has an address of its own, so the browser's bar shows the link that can be sent. It is
  // put there with replaceState: that makes no hashchange, so the page stays the game's (Root.jsx and the
  // game pages only look at the hash when it changes). When the review is closed, the game's address returns.
  const doneId = job?.status === "done" && job.report ? job.id : "";
  useEffect(() => {
    if (!doneId) return undefined;
    const before = window.location.hash;
    const mine = `#review/${doneId}`;
    window.history.replaceState(null, "", mine);
    return () => {
      if (window.location.hash === mine) window.history.replaceState(null, "", before || window.location.pathname + window.location.search);
    };
  }, [doneId]);

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
          <h2>{error ? t("review.failed") : t("reviewInline.2")}</h2>
          {error ? (
            <div className="banner banner--error">{error}</div>
          ) : (
            <div className="progress" aria-live="polite">
              <div className="progress__bar" style={{ width: `${busy ? Math.max(pct, 3) : 100}%` }} />
              <span>
                {!job || job.status === "queued" ? t("reviewInline.3") : t("reviewInline.4", { done: job.done, total: job.total })}
              </span>
            </div>
          )}
          <p className="review__hint">{t("reviewInline.5")}</p>
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            <Icon name="back" size={16} /> {t("reviewInline.6")}
          </button>
        </div>
      </aside>
    </div>
  );
}

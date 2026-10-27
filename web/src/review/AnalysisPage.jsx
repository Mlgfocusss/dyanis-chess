import { useEffect, useState } from "react";
import ReviewView from "./ReviewView.jsx";
import { fetchLine } from "./api.js";
import { useAnalysis } from "./useAnalysis.js";
import { encodeFen, fenFromHash, looksLikeFen } from "./moments.js";
import { START_FEN } from "../play/game.js";
import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";
import "./review.css";

// "Доска анализа" (#analysis, #analysis/<position>): a board with no game. Move any piece from any
// position, the engine shows its best lines and judges each move; no review has to be made first.
// It is the same panel as a review's (ReviewView with free). A position can be loaded in the second
// tab; a game's moves (PGN) go to a review instead, which opens when it is ready.
// A position gets its own address (the "Поделиться" menu copies it), and a review's panel can open
// the position on the board here.
export default function AnalysisPage() {
  const t = useT();
  const [fen, setFen] = useState(() => fenFromHash(window.location.hash) || START_FEN);
  const [seed, setSeed] = useState(null); // the position as a review of a game with no moves
  const [error, setError] = useState("");

  useEffect(() => {
    const onHash = () => {
      setError("");
      setFen(fenFromHash(window.location.hash) || START_FEN);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    let alive = true;
    setSeed(null);
    fetchLine({ fen, eval: true })
      .then((r) => {
        if (!alive) return;
        setSeed({
          id: "",
          status: "done",
          game: { white: "", black: "", result: "*", startFen: r.fen },
          report: {
            evals: [{ ...(r.eval || { score: 0, winPct: 50, depth: 0 }), ply: 0, fen: r.fen }],
            moves: [],
            accuracy: [0, 0],
            counts: [{}, {}],
            key: [],
            options: { depth: 0 },
          },
        });
      })
      .catch((e) => {
        if (!alive) return;
        setError(e.message);
        if (fen !== START_FEN) setFen(START_FEN); // a wrong position falls back to the start
      });
    return () => {
      alive = false;
    };
  }, [fen]);

  return (
    <div className="page page--stage">
      {error && <div className="banner banner--error">{error}</div>}
      {seed ? (
        <ReviewView key={fen} job={seed} free loader={<Loader />} />
      ) : (
        !error && <p className="review__hint">{t("analysisPage.1")}</p>
      )}
    </div>
  );
}

// What can be put on the board: a position (FEN), or a game (PGN), which is reviewed.
function Loader() {
  const t = useT();
  const [text, setText] = useState("");
  const [msg, setMsg] = useState("");
  const { job, error, busy, start } = useAnalysis();

  // the review of a pasted game is ready: open it
  useEffect(() => {
    if (job?.status === "done") window.location.hash = `review/${job.id}`;
  }, [job]);

  function load() {
    const src = text.trim();
    if (!src) return;
    setMsg("");
    if (looksLikeFen(src)) {
      window.location.hash = `analysis/${encodeFen(src)}`;
      setText("");
    } else if (/[a-h][1-8]|O-O/.test(src)) {
      start({ pgn: src, depth: 14 }); // a game: the server reviews it, then the review opens
    } else {
      setMsg(t("analysisPage.2"));
    }
  }

  return (
    <div className="loader">
      <p className="review__hint">
        {t("analysisPage.3")}
      </p>
      <textarea
        className="field field--area loader__area"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={t("analysisPage.4")}
        rows={6}
        spellCheck={false}
        disabled={busy}
      />
      <div className="review__row">
        <button type="button" className="btn btn--primary btn--small" onClick={load} disabled={busy || !text.trim()}>
          <Icon name="upload" size={14} /> {busy ? t("analysisPage.5") : t("analysisPage.6")}
        </button>
        <button
          type="button"
          className="btn btn--small btn--ghost"
          onClick={() => (window.location.hash = "analysis")}
          disabled={busy}
        >
          {t("analysisPage.7")}
        </button>
      </div>
      {busy && job && job.total > 0 && (
        <div className="progress" aria-live="polite">
          <div className="progress__bar" style={{ width: `${Math.round((100 * job.done) / job.total)}%` }} />
          <span>{t("analysisPage.8")} {job.done} {t("analysisPage.9")} {job.total}</span>
        </div>
      )}
      {(msg || error) && <div className="banner banner--error">{msg || error}</div>}
    </div>
  );
}

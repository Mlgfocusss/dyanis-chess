import { useEffect, useRef, useState } from "react";
import { createAnalysis, getAnalysis } from "./api.js";
import ReviewView from "./ReviewView.jsx";
import Icon from "../ui/Icon.jsx";
import "./review.css";

const POLL_MS = 400;
const SAMPLE = `[White "Анна"]
[Black "Борис"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 Nf6 4. Ng5 d5 5. exd5 Nxd5 6. Nxf7 Kxf7 7. Qf3+ Ke6 8. Nc3 Ncb4 9. O-O c6 10. d4 *`;

// "Разбор партии": paste a PGN, wait for the server, browse the result.
// The address of a finished review is #review/<id>: the server keeps the
// result, so the link works after a reload and for anyone it is sent to.
function idFromHash() {
  const [tab, id] = window.location.hash.slice(1).split("/");
  return tab === "review" && id ? id : "";
}

export default function ReviewPage() {
  const [pgn, setPgn] = useState("");
  const [depth, setDepth] = useState(14);
  const [job, setJob] = useState(null); // last state from the server
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const timer = useRef(null);

  useEffect(() => {
    const id = idFromHash();
    if (id) {
      setBusy(true);
      poll(id);
    }
    return () => clearTimeout(timer.current);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function start() {
    setError("");
    setJob(null);
    setBusy(true);
    try {
      const id = await createAnalysis({ pgn, depth });
      poll(id);
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  async function poll(id) {
    try {
      const j = await getAnalysis(id);
      setJob(j);
      if (j.status === "done") {
        window.history.replaceState(null, "", `#review/${id}`);
        return setBusy(false);
      }
      if (j.status === "error") {
        setError(j.error || "Разбор не удался");
        return setBusy(false);
      }
      timer.current = setTimeout(() => poll(id), POLL_MS);
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  function onFile(e) {
    const f = e.target.files?.[0];
    if (!f) return;
    f.text().then(setPgn);
    e.target.value = "";
  }

  const done = job?.status === "done" && job.report;
  const pct = job && job.total ? Math.round((100 * job.done) / job.total) : 0;

  return (
    <div className="page">
      {!done && (
        <section className="review-form">
          <header className="page__head">
            <h1 className="page__title">Разбор партии</h1>
            <p className="page__sub">
              Вставьте партию в формате PGN (например, скачанную с chess.com или lichess): движок оценит
              каждый ход, найдёт ошибки и назовёт дебют.
            </p>
          </header>
          <textarea
            className="field field--area"
            value={pgn}
            onChange={(e) => setPgn(e.target.value)}
            placeholder={'[White "…"]\n[Black "…"]\n\n1. e4 e5 2. Nf3 …'}
            rows={12}
            spellCheck={false}
            disabled={busy}
          />
          <div className="review-form__row">
            <button type="button" className="btn btn--primary" onClick={start} disabled={busy || !pgn.trim()}>
              {busy ? "Идёт разбор…" : "Разобрать"}
            </button>
            <label className="btn btn--ghost">
              <Icon name="upload" size={16} />
              Загрузить файл
              <input type="file" accept=".pgn,.txt" onChange={onFile} hidden disabled={busy} />
            </label>
            <button type="button" className="btn btn--ghost" onClick={() => setPgn(SAMPLE)} disabled={busy}>
              Пример
            </button>
            <label className="review-form__depth">
              глубина
              <select className="field" value={depth} onChange={(e) => setDepth(Number(e.target.value))} disabled={busy}>
                <option value={10}>10 — быстро</option>
                <option value={14}>14 — обычно</option>
                <option value={18}>18 — точнее</option>
              </select>
            </label>
          </div>
          {busy && (
            <div className="progress" aria-live="polite">
              <div className="progress__bar" style={{ width: `${job ? pct : 3}%` }} />
              <span>
                {!job || job.status === "queued"
                  ? "В очереди…"
                  : `Позиций оценено: ${job.done} из ${job.total}`}
              </span>
            </div>
          )}
          {error && <div className="banner banner--error">{error}</div>}
        </section>
      )}

      {done && (
        <>
          <div className="page__bar">
            <button
              type="button"
              className="btn btn--ghost"
              onClick={() => {
                setJob(null);
                setError("");
                window.history.replaceState(null, "", "#review");
              }}
            >
              <Icon name="back" size={16} />
              Другая партия
            </button>
          </div>
          <ReviewView key={job.id} job={job} />
        </>
      )}
    </div>
  );
}

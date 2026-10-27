import { useEffect, useRef, useState } from "react";
import { useAnalysis } from "./useAnalysis.js";
import ReviewView from "./ReviewView.jsx";
import Icon from "../ui/Icon.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useT } from "../i18n/index.js";
import "./review.css";

const SAMPLE = `[White "Анна"]
[Black "Борис"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 Nf6 4. Ng5 d5 5. exd5 Nxd5 6. Nxf7 Kxf7 7. Qf3+ Ke6 8. Nc3 Ncb4 9. O-O c6 10. d4 *`;

// "Разбор партии": paste a PGN, wait for the server, browse the result.
// The address of a finished review is #review/<id>: the server keeps the
// result, so the link works after a reload and for anyone it is sent to.
// #review/<id>/<n> opens it on the n-th half-move (the "copy link" of the panel adds it).
function fromHash() {
  const [tab, id, n] = window.location.hash.slice(1).split("/");
  if (tab !== "review" || !id) return { id: "", ply: 0 };
  const ply = Number.parseInt(n, 10);
  return { id, ply: ply > 0 ? ply : 0 };
}

export default function ReviewPage() {
  const t = useT();
  const { status: authStatus, user } = useAuth();
  const [pgn, setPgn] = useState("");
  const [depth, setDepth] = useState(14);
  const { job, error, busy, start, attach, reset } = useAnalysis();

  // the review and the move a link pointed at: used for that review only
  const [linked, setLinked] = useState(fromHash);
  const shownId = useRef("");
  shownId.current = job?.id || "";

  // The address changed by hand or by a link while this page is open (another review pasted into the
  // bar, the menu item pressed): the page follows it. replaceState makes no such event.
  useEffect(() => {
    const onHash = () => {
      const l = fromHash();
      if (l.id && l.id !== shownId.current) {
        setLinked(l);
        reset();
        attach(l.id);
      } else if (!l.id && shownId.current) reset(); // "#review": the form for a new one
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const { id } = fromHash();
    if (id) attach(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // A finished review gets its own address, so a reload or a shared link finds it.
  useEffect(() => {
    if (job?.status === "done" && fromHash().id !== job.id) window.history.replaceState(null, "", `#review/${job.id}`);
  }, [job]);

  function startReview() {
    start({ pgn, depth });
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
    <div className={"page" + (done ? " page--stage" : "")}>
      {!done && (
        <section className="review-form">
          <header className="page__head">
            <h1 className="page__title">{t("menu.review")}</h1>
            <p className="page__sub">
              {t("reviewPage.3")}
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
            <button type="button" className="btn btn--primary" onClick={startReview} disabled={busy || !pgn.trim()}>
              {busy ? t("analysisPage.5") : t("reviewPage.5")}
            </button>
            <label className="btn btn--ghost">
              <Icon name="upload" size={16} />
              {t("reviewPage.6")}
              <input type="file" accept=".pgn,.txt" onChange={onFile} hidden disabled={busy} />
            </label>
            <button type="button" className="btn btn--ghost" onClick={() => setPgn(SAMPLE)} disabled={busy}>
              {t("reviewPage.7")}
            </button>
            <label className="review-form__depth">
              {t("reviewPage.8")}
              <select className="field" value={depth} onChange={(e) => setDepth(Number(e.target.value))} disabled={busy}>
                <option value={10}>{t("reviewPage.9")}</option>
                <option value={14}>{t("reviewPage.10")}</option>
                <option value={18}>{t("reviewPage.11")}</option>
              </select>
            </label>
          </div>
          {busy && (
            <div className="progress" aria-live="polite">
              <div className="progress__bar" style={{ width: `${job ? pct : 3}%` }} />
              <span>
                {!job || job.status === "queued"
                  ? t("reviewInline.3")
                  : t("reviewPage.13", { done: job.done, total: job.total })}
              </span>
            </div>
          )}
          <p className="review-form__note">
            {t("reviewPage.14")} <a href="#analysis">{t("reviewPage.15")}</a>.
          </p>
          {authStatus === "ready" && !user && (
            <p className="review-form__note">
              {t("reviewPage.16")}{" "}
              <a href="#account">{t("menu.signIn")}</a>
            </p>
          )}
          {error && <div className="banner banner--error">{error}</div>}
        </section>
      )}

      {done && (
        <ReviewView
          key={job.id}
          job={job}
          startPly={job.id === linked.id ? linked.ply : 0}
          onNew={() => {
            reset();
            window.history.replaceState(null, "", "#review");
          }}
        />
      )}
    </div>
  );
}

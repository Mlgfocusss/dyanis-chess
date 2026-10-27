import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { closeReport, fetchReports } from "../community/api.js";
import { ago, forumHash, reportActions, reportLink } from "../community/logic.js";
import "../community/community.css";
import { useT } from "../i18n/index.js";

// #forum/reports: the queue of complaints for the moderators (the ids in FORUM_ADMINS). Each complaint
// shows the text as it was when the complaint was made, and is closed with or without an action.
export default function ReportsQueue() {
  const t = useT();
  const { user } = useAuth();
  const [status, setStatus] = useState("open");
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [denied, setDenied] = useState(false);

  const load = useCallback(() => {
    fetchReports(status).then(
      (list) => {
        setItems(list);
        setError("");
      },
      (e) => {
        if (e.status === 403 || e.status === 401) setDenied(true);
        else setError(e.message);
      },
    );
  }, [status]);
  useEffect(() => {
    setItems(null);
    load();
  }, [load]);

  const crumbs = (
    <nav className="fcrumbs" aria-label={t("forumPage.3")}>
      <a href={forumHash.home()}>{t("menu.forum")}</a>
      <span aria-hidden="true"> › </span>
      <span>{t("safety.crumb")}</span>
    </nav>
  );
  if (denied || !user) {
    return (
      <>
        {crumbs}
        <div className="banner banner--error">{t("safety.noAccess")}</div>
      </>
    );
  }

  async function act(report, action) {
    if (action === "delete" && !window.confirm(t("safety.confirmDelete"))) return;
    try {
      await closeReport(report.id, action);
      load();
    } catch (e) {
      setError(e.message);
    }
  }

  const now = Date.now();
  return (
    <>
      {crumbs}
      <section className="card csection">
        <header className="csection__head">
          <h2>{t("safety.queueTitle")}</h2>
          <span className="ctabs" role="tablist">
            {["open", "closed"].map((s) => (
              <button
                key={s}
                type="button"
                role="tab"
                aria-selected={status === s}
                className={status === s ? "btn btn--primary" : "btn btn--ghost"}
                onClick={() => setStatus(s)}
              >
                {t(`safety.${s}`)}
              </button>
            ))}
          </span>
        </header>
        <p className="cnote">{t("safety.queueIntro")}</p>
        {error && <div className="banner banner--error">{error}</div>}
        {!items && !error && <p className="cnote">{t("safety.loading")}</p>}
        {items && items.length === 0 && <p className="cnote">{status === "open" ? t("safety.queueEmpty") : t("safety.queueEmptyClosed")}</p>}
      </section>
      <div className="freports">
        {items?.map((r) => (
          <ReportCard key={r.id} report={r} now={now} onAct={act} />
        ))}
      </div>
    </>
  );
}

function who(p, t) {
  if (!p.name) return t("safety.accountGone");
  return p.id ? <a href={`#user/${p.id}`}>{p.name}</a> : <span>{p.name}</span>;
}

function ReportCard({ report: r, now, onAct }) {
  const t = useT();
  const link = reportLink(r);
  return (
    <article className="card freport">
      <header className="freport__head">
        <span className="freport__kind">{t(`safety.kind.${r.kind}`)}</span>
        <span className="freport__reason">{t(`safety.reason.${r.reason}`)}</span>
        {link && (
          <a className="btn btn--ghost" href={link}>
            {t("safety.goTo")}
          </a>
        )}
        <time className="freport__time">{ago(r.created, now)}</time>
      </header>
      <p className="freport__who">
        {t("safety.from")}: {who(r.reporter, t)} · {t("safety.about")}: {who(r.target, t)}
      </p>
      {r.note && <p className="freport__note">{r.note}</p>}
      {/* plain text, as it was when the complaint was made */}
      <p className="freport__text">{r.snapshot}</p>
      {r.status === "open" ? (
        <div className="freport__actions">
          {reportActions(r.kind).map((a) => (
            <button key={a} type="button" className={`btn${a === "delete" ? " btn--danger-ghost" : a === "none" ? " btn--primary" : ""}`} onClick={() => onAct(r, a)}>
              {a === "delete" ? t(`safety.act.delete.${r.kind}`) : t(`safety.act.${a}`)}
            </button>
          ))}
        </div>
      ) : (
        <p className="cnote">{t(`safety.done.${r.action || "none"}`)}</p>
      )}
    </article>
  );
}

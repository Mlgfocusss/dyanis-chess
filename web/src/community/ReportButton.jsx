import { useEffect, useRef, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useT } from "../i18n/index.js";
import Icon from "../ui/Icon.jsx";
import { sendReport } from "./api.js";
import { REPORT_NOTE_MAX, REPORT_REASONS, targetOf } from "./logic.js";
import "./community.css";

// "Пожаловаться": a button that opens the form of a complaint on a private message, a forum message or
// topic, or a player. `kind` is the server's ("message", "forum_post", "forum_topic", "user"), `targetId`
// is what it is about. Shown only to a signed-in person. After the complaint is sent the button turns
// into a note, so that the same thing is not reported twice.
export default function ReportButton({ kind, targetId, className = "btn btn--ghost", icon = true }) {
  const t = useT();
  const { user } = useAuth();
  const [open, setOpen] = useState(false);
  const [done, setDone] = useState(false);

  if (!user) return null;
  if (done) return <span className="cnote creport__done">{t("safety.sent")}</span>;
  return (
    <>
      <button type="button" className={className} onClick={() => setOpen(true)}>
        {icon && <Icon name="alert" size={16} />} {t("safety.report")}
      </button>
      {open && (
        <ReportForm
          kind={kind}
          targetId={targetId}
          onClose={() => setOpen(false)}
          onSent={() => {
            setOpen(false);
            setDone(true);
          }}
        />
      )}
    </>
  );
}

function ReportForm({ kind, targetId, onClose, onSent }) {
  const t = useT();
  const [reason, setReason] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const first = useRef(null);

  useEffect(() => {
    first.current?.focus();
    const onKey = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function send() {
    setBusy(true);
    setError("");
    try {
      await sendReport({ kind, targetId: targetOf(targetId), reason, note: note.trim() });
      onSent();
    } catch (e) {
      if (e.status === 409) {
        // already reported: the moderator has it
        onSent();
        return;
      }
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <div className="cmodal" role="presentation" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="cmodal__box card" role="dialog" aria-modal="true" aria-labelledby="report-title">
        <header className="cform__head">
          <h3 id="report-title">{t(`safety.reportTitle.${kind}`)}</h3>
          <button type="button" className="btn btn--ghost" onClick={onClose} aria-label={t("safety.close")}>
            <Icon name="close" size={16} />
          </button>
        </header>
        <fieldset className="creport__reasons">
          <legend className="cnote">{t("safety.reasonLabel")}</legend>
          {REPORT_REASONS.map((r, i) => (
            <label key={r} className={`creport__reason${reason === r ? " creport__reason--on" : ""}`}>
              <input ref={i === 0 ? first : null} type="radio" name="report-reason" value={r} checked={reason === r} onChange={() => setReason(r)} />
              {t(`safety.reason.${r}`)}
            </label>
          ))}
        </fieldset>
        <label className="cnote" htmlFor="report-note">
          {t("safety.noteLabel")}
        </label>
        <textarea
          id="report-note"
          className="field creport__note"
          rows={3}
          maxLength={REPORT_NOTE_MAX}
          value={note}
          placeholder={t("safety.notePlaceholder", { max: REPORT_NOTE_MAX })}
          onChange={(e) => setNote(e.target.value)}
        />
        <p className="cnote">{t("safety.reportHint")}</p>
        {error && <div className="banner banner--error">{error}</div>}
        <div className="creport__bar">
          <button type="button" className="btn btn--ghost" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="button" className="btn btn--primary" disabled={busy || !reason} onClick={send}>
            {busy ? t("safety.sending") : t("safety.send")}
          </button>
        </div>
      </div>
    </div>
  );
}

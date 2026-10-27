import { useEffect, useState } from "react";
import BotAvatar from "../bots/BotAvatar.jsx";
import Icon from "../ui/Icon.jsx";
import { askCoach, coachEnabled } from "./api.js";
import { isCostly } from "./dionis.js";
import { useT } from "../i18n/index.js";

const DIONIS = { id: "dionis", name: "Дионис Самусенко" };

// Dionis, the coach of the platform, beside the board of a review. He always has a short remark on the
// move (brief: { text, best }, made by dionis.js, free and at once) and the arrows he draws on the board;
// "Лучший ход" plays the engine's move instead of a costly one. "Подробнее" asks the AI coach (the
// language model of the server) for a longer explanation, only when it is wanted, because each is a call
// to a model. The AI text is the model's, the lines are the engine's; a line plays out on the board.
// ply: the move of the game it is about (0: the start); note: the explanation already received for
// it, or null; onNote(ply, note) keeps it; onShow(basePly, sans, title) plays sans from the position
// after basePly half-moves; onHide turns the coach off.
export default function DionisCard({ jobId, ply, move, brief, note, onNote, onShow, onHide }) {
  const t = useT();
  const [aiOn, setAiOn] = useState(false);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    coachEnabled().then((v) => alive && setAiOn(v));
    return () => {
      alive = false;
    };
  }, []);
  // another move: its own state
  useEffect(() => {
    setBusy(false);
    setError("");
    setOpen(false);
  }, [ply]);

  async function ask() {
    setOpen(true);
    if (note) return;
    setBusy(true);
    setError("");
    const asked = ply;
    try {
      onNote(asked, await askCoach(jobId, asked));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  const costly = move && isCostly(move.class);
  return (
    <section className="dionis card" aria-label={t("dionisCard.2")}>
      <div className="dionis__top">
        <BotAvatar bot={DIONIS} size={32} />
        <div className="dionis__col">
          <p className="dionis__say" title={t("dionisCard.2")}>
            {brief.text}
          </p>
          {move && ((costly && brief.best) || (aiOn && !open)) && (
          <div className="dionis__row">
            {move && costly && brief.best && (
              <button type="button" className="btn btn--small" onClick={() => onShow(ply - 1, [brief.best], t("dionisCard.3"))}>
                {t("dionisCard.4")}
              </button>
            )}
            {move && aiOn && !open && (
              <button type="button" className="btn btn--small btn--ghost" onClick={ask}>
                <Icon name="coach" size={14} /> {t("dionisCard.5")}
              </button>
            )}
          </div>
          )}
        </div>
      </div>

      <button type="button" className="dionis__hide" onClick={onHide} title={t("dionisCard.6")} aria-label={t("dionisCard.7")}>
        <Icon name="close" size={14} />
      </button>

      {open && (
        <div className="dionis__more">
          {busy && <p className="coach__lead coach__busy">{t("dionisCard.8")}</p>}
          {error && <div className="banner banner--error">{error}</div>}
          {note && (
            <div className="coach__body">
              {note.text.split(/\n+/).map((p, i) => (
                <p key={i}>{p}</p>
              ))}
              <div className="coach__show">
                {costly && note.bestLine?.length > 0 && (
                  <button type="button" className="btn" onClick={() => onShow(ply - 1, note.bestLine, t("dionisCard.9"))}>
                    {t("dionisCard.10")}
                  </button>
                )}
                {costly && note.replyLine?.length > 0 && (
                  <button type="button" className="btn" onClick={() => onShow(ply, note.replyLine, t("dionisCard.11"))}>
                    {t("dionisCard.12")}
                  </button>
                )}
                {!costly && note.replyLine?.length > 0 && (
                  <button type="button" className="btn" onClick={() => onShow(ply, note.replyLine, t("dionisCard.13"))}>
                    {t("dionisCard.14")}
                  </button>
                )}
              </div>
              <p className="coach__note">{t("dionisCard.15")}</p>
            </div>
          )}
          {!busy && !note && error && (
            <button type="button" className="btn btn--ghost" onClick={ask}>
              {t("dionisCard.16")}
            </button>
          )}
        </div>
      )}
    </section>
  );
}

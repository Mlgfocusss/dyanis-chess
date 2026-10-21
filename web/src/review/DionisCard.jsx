import { useEffect, useState } from "react";
import BotAvatar from "../bots/BotAvatar.jsx";
import Icon from "../ui/Icon.jsx";
import { askCoach, coachEnabled } from "./api.js";
import { isCostly } from "./dionis.js";

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
    <section className="dionis card" aria-label="Тренер Дионис">
      <div className="dionis__top">
        <BotAvatar bot={DIONIS} size={32} />
        <div className="dionis__col">
          <p className="dionis__say" title="Тренер Дионис">
            {brief.text}
          </p>
          {move && ((costly && brief.best) || (aiOn && !open)) && (
          <div className="dionis__row">
            {move && costly && brief.best && (
              <button type="button" className="btn btn--small" onClick={() => onShow(ply - 1, [brief.best], "Дионис: лучший ход")}>
                Лучший ход
              </button>
            )}
            {move && aiOn && !open && (
              <button type="button" className="btn btn--small btn--ghost" onClick={ask}>
                <Icon name="coach" size={14} /> Подробнее
              </button>
            )}
          </div>
          )}
        </div>
      </div>

      <button type="button" className="dionis__hide" onClick={onHide} title="Скрыть тренера (вернуть: Настройки, Игровой процесс)" aria-label="Скрыть тренера">
        <Icon name="close" size={14} />
      </button>

      {open && (
        <div className="dionis__more">
          {busy && <p className="coach__lead coach__busy">Разбираю позицию подробнее…</p>}
          {error && <div className="banner banner--error">{error}</div>}
          {note && (
            <div className="coach__body">
              {note.text.split(/\n+/).map((p, i) => (
                <p key={i}>{p}</p>
              ))}
              <div className="coach__show">
                {costly && note.bestLine?.length > 0 && (
                  <button type="button" className="btn" onClick={() => onShow(ply - 1, note.bestLine, "Дионис: что нужно было сыграть")}>
                    Что нужно было сыграть
                  </button>
                )}
                {costly && note.replyLine?.length > 0 && (
                  <button type="button" className="btn" onClick={() => onShow(ply, note.replyLine, "Дионис: как это наказывается")}>
                    Как это наказывается
                  </button>
                )}
                {!costly && note.replyLine?.length > 0 && (
                  <button type="button" className="btn" onClick={() => onShow(ply, note.replyLine, "Дионис: что дальше")}>
                    Показать продолжение
                  </button>
                )}
              </div>
              <p className="coach__note">Подробный текст пишет нейросеть по данным движка, ходы в линиях считал сам движок.</p>
            </div>
          )}
          {!busy && !note && error && (
            <button type="button" className="btn btn--ghost" onClick={ask}>
              Ещё раз
            </button>
          )}
        </div>
      )}
    </section>
  );
}

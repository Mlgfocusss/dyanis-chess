import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n/index.js";
import "./online.css";

const MAX = 200;

function clock(ms) {
  const d = new Date(ms);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

// The chat of a game between two people: the lines, and a box for the player's
// own. Spectators read; only the players write. The server cleans and limits
// the text; here it is only ever shown as text.
export default function Chat({ game, you, send }) {
  const t = useT();
  const [text, setText] = useState("");
  const end = useRef(null);
  const lines = game.chat || [];

  useEffect(() => {
    end.current?.scrollIntoView?.({ block: "nearest" });
  }, [lines.length]);

  function submit(e) {
    e.preventDefault();
    const t = text.trim();
    if (!t) return;
    if (send("chat", { text: t })) setText("");
  }

  return (
    <section className="card chat">
      <h2>{t("chat.1")}</h2>
      <div className="chat__lines" aria-live="polite">
        {lines.length === 0 && <p className="chat__empty">{t("chat.2")}</p>}
        {lines.map((m, i) => (
          <p key={i} className={`chat__line chat__line--${m.side}${m.side === you ? " chat__line--mine" : ""}`}>
            <span className="chat__who">{game[m.side]?.name || (m.side === "white" ? t("online.white") : t("online.black"))}</span>
            <span className="chat__time">{clock(m.at)}</span>
            <span className="chat__text">{m.text}</span>
          </p>
        ))}
        <div ref={end} />
      </div>
      {you ? (
        <form className="chat__form" onSubmit={submit}>
          <input
            className="field"
            value={text}
            maxLength={MAX}
            onChange={(e) => setText(e.target.value)}
            placeholder={t("chat.5")}
            aria-label={t("chat.6")}
          />
          <button type="submit" className="btn" disabled={!text.trim()}>
            {t("chat.7")}
          </button>
        </form>
      ) : (
        <p className="chat__hint">{t("chat.8")}</p>
      )}
    </section>
  );
}

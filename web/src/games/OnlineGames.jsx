import { useEffect, useState } from "react";
import { createAnalysis } from "../review/api.js";
import { describeTC } from "../online/clock.js";
import { fetchPgn, listGames } from "../online/api.js";
import { locale, useT, tr } from "../i18n/index.js";
import "./games.css";

const REVIEW_DEPTH = 14;

function outcome(g) {
  if (g.status === "playing") return { text: tr("onlineGames.1"), cls: "live" };
  if (g.result === "1/2-1/2") return { text: tr("onlineGames.2"), cls: "draw" };
  const won = (g.result === "1-0" && g.you === "white") || (g.result === "0-1" && g.you === "black");
  return won ? { text: tr("onlineGames.3"), cls: "win" } : { text: tr("onlineGames.4"), cls: "loss" };
}

// The games played against other people, in the profile's "Партии" tab.
export default function OnlineGames() {
  const t = useT();
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  useEffect(() => {
    let alive = true;
    listGames().then(
      (list) => alive && setItems(list),
      (e) => alive && setError(e.message)
    );
    return () => {
      alive = false;
    };
  }, []);

  async function review(g) {
    setBusy(g.id);
    setError("");
    try {
      const id = await createAnalysis({ pgn: await fetchPgn(g.id), depth: REVIEW_DEPTH });
      window.location.hash = `review/${id}`;
    } catch (e) {
      setError(e.message);
      setBusy("");
    }
  }

  if (!items && !error) return null;
  return (
    <section className="games__section">
      <h2 className="games__h">{t("onlineGames.5")}</h2>
      {error && <div className="banner banner--error">{error}</div>}
      {items && items.length === 0 && (
        <section className="card games__empty">
          <p>{t("onlineGames.6")}</p>
          <a className="btn btn--primary" href="#online">
            {t("home.playOnline")}
          </a>
        </section>
      )}
      {items && items.length > 0 && (
        <ul className="card games__list">
          {items.map((g) => {
            const res = outcome(g);
            const opp = g.you === "white" ? g.black : g.white;
            return (
              <li key={g.id} className="games__row">
                <a className="games__link" href={`#online/${g.id}`}>
                  <span className={`games__result games__result--${res.cls}`}>{res.text}</span>
                  <span className="games__main">
                    <span className="games__title">
                      {g.you === "white" ? t("onlineGames.8") : t("onlineGames.9")} {t("onlineGames.10")} {opp?.name || t("onlineGames.11")}
                    </span>
                    <span className="games__sub">
                      {new Date(g.updated).toLocaleDateString(locale(), { day: "numeric", month: "short", year: "numeric" })}
                      {t("onlineGames.12", { p: describeTC(g.tc), p2: Math.ceil(g.plies / 2) })}
                    </span>
                  </span>
                </a>
                {g.status === "ended" && (
                  <button type="button" className="btn btn--ghost games__review" disabled={busy === g.id || g.plies === 0} onClick={() => review(g)}>
                    {busy === g.id ? "…" : t("onlineGames.review")}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

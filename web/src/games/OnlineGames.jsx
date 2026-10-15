import { useEffect, useState } from "react";
import { createAnalysis } from "../review/api.js";
import { describeTC } from "../online/clock.js";
import { fetchPgn, listGames } from "../online/api.js";
import "./games.css";

const REVIEW_DEPTH = 14;

function outcome(g) {
  if (g.status === "playing") return { text: "идёт", cls: "live" };
  if (g.result === "1/2-1/2") return { text: "Ничья", cls: "draw" };
  const won = (g.result === "1-0" && g.you === "white") || (g.result === "0-1" && g.you === "black");
  return won ? { text: "Победа", cls: "win" } : { text: "Поражение", cls: "loss" };
}

// The games played against other people, in the profile's "Партии" tab.
export default function OnlineGames() {
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
      <h2 className="games__h">Онлайн-партии</h2>
      {error && <div className="banner banner--error">{error}</div>}
      {items && items.length === 0 && (
        <section className="card games__empty">
          <p>Вы ещё не играли с людьми. Создайте партию и отправьте ссылку другу.</p>
          <a className="btn btn--primary" href="#online">
            Играть онлайн
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
                      {g.you === "white" ? "Белыми" : "Чёрными"} против {opp?.name || "соперника"}
                    </span>
                    <span className="games__sub">
                      {new Date(g.updated).toLocaleDateString("ru-RU", { day: "numeric", month: "short", year: "numeric" })}
                      {` · ${describeTC(g.tc)} · ходов: ${Math.ceil(g.plies / 2)}`}
                    </span>
                  </span>
                </a>
                {g.status === "ended" && (
                  <button type="button" className="btn btn--ghost games__review" disabled={busy === g.id || g.plies === 0} onClick={() => review(g)}>
                    {busy === g.id ? "…" : "Разбор"}
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

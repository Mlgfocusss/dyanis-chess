import { useEffect, useState } from "react";
import { describeTC } from "../online/clock.js";
import { listGames } from "./api.js";
import "../games/games.css";

// the teams of a game of four: seats 0 and 3 (result "1-0") against seats 1 and 2
const teamOfSeat = (seat) => (seat === 0 || seat === 3 ? 0 : 1);

function outcome(g) {
  if (g.status === "playing") return { text: "идёт", cls: "live" };
  if (g.result === "1/2-1/2") return { text: "Ничья", cls: "draw" };
  if (g.players) return g.result === (teamOfSeat(g.seat) === 0 ? "1-0" : "0-1") ? { text: "Победа", cls: "win" } : { text: "Поражение", cls: "loss" };
  const won = (g.result === "1-0" && g.you === "white") || (g.result === "0-1" && g.you === "black");
  return won ? { text: "Победа", cls: "win" } : { text: "Поражение", cls: "loss" };
}

// The games of the chess variants ("Другие шахматы"), in the profile's "Партии" tab. Nothing is shown to
// someone who has played none.
export default function VariantGames() {
  const [items, setItems] = useState(null);

  useEffect(() => {
    let alive = true;
    listGames().then(
      (list) => alive && setItems(list),
      () => {}
    );
    return () => {
      alive = false;
    };
  }, []);

  if (!items || items.length === 0) return null;
  return (
    <section className="games__section">
      <h2 className="games__h">Другие шахматы</h2>
      <ul className="card games__list">
        {items.map((g) => {
          const res = outcome(g);
          const opp = g.you === "white" ? g.black : g.white;
          const names = (seats) => seats.map((s) => g.players?.[s]?.name || "—").join(" и ");
          const rivals = g.players ? names([0, 1, 2, 3].filter((s) => teamOfSeat(s) !== teamOfSeat(g.seat))) : "";
          const partner = g.players ? g.players[3 - g.seat]?.name || "—" : "";
          return (
            <li key={g.id} className="games__row">
              <a className="games__link" href={`#variants/game/${g.id}`}>
                <span className={`games__result games__result--${res.cls}`}>{res.text}</span>
                <span className="games__main">
                  <span className="games__title">
                    {g.players
                      ? `${g.variantName || g.variant}: с напарником ${partner} против ${rivals}`
                      : `${g.variantName || g.variant}: ${g.you === "white" ? "белыми" : "чёрными"} против ${opp?.name || "соперника"}`}
                  </span>
                  <span className="games__sub">
                    {new Date(g.updated).toLocaleDateString("ru-RU", { day: "numeric", month: "short", year: "numeric" })}
                    {` · ${describeTC(g.tc)} · ходов: ${Math.ceil(g.plies / 2)}`}
                  </span>
                </span>
              </a>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

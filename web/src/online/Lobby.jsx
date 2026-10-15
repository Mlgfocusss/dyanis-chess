import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Segmented from "../ui/Segmented.jsx";
import { createGame } from "./api.js";
import { TIME_CONTROLS, tcToRequest } from "./clock.js";
import "./online.css";

const COLORS = [
  ["white", "Белые"],
  ["random", "Случайно"],
  ["black", "Чёрные"],
];

// "Онлайн": make a game and get a link for a friend.
export default function Lobby() {
  const { status, user, guest } = useAuth();
  const [tc, setTc] = useState([5, 0]);
  const [color, setColor] = useState("random");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function create() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest(); // one click: the game needs a person, a guest will do
      const id = await createGame({ color, ...tcToRequest(tc) });
      window.location.hash = `online/${id}`;
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  const same = (a) => a[0] === tc[0] && a[1] === tc[1];

  return (
    <div className="page lobby">
      <header className="page__head">
        <h1 className="page__title">Игра с другом</h1>
        <p className="page__sub">
          Создайте партию и отправьте ссылку: соперник откроет её и сядет играть. Регистрация не нужна.
        </p>
      </header>

      {status === "unavailable" && (
        <div className="banner banner--error">Онлайн-игра недоступна: сервер запущен без базы данных.</div>
      )}
      {error && <div className="banner banner--error">{error}</div>}

      <section className="card lobby__card">
        <h2 className="lobby__h">Время на партию</h2>
        {TIME_CONTROLS.map((g) => (
          <div key={g.group} className="lobby__group">
            <span className="lobby__gname">{g.group}</span>
            <div className="lobby__opts">
              {g.options.map((o) => (
                <button
                  key={o.join("+")}
                  type="button"
                  className={`chip${same(o) ? " chip--on" : ""}`}
                  aria-pressed={same(o)}
                  onClick={() => setTc(o)}
                >
                  {o[1] ? `${o[0]} + ${o[1]}` : `${o[0]} мин`}
                </button>
              ))}
            </div>
          </div>
        ))}

        <h2 className="lobby__h">Ваш цвет</h2>
        <Segmented label="Цвет" value={color} onChange={setColor} options={COLORS} />

        <button
          type="button"
          className="btn btn--primary lobby__go"
          onClick={create}
          disabled={busy || status === "unavailable" || status === "loading"}
        >
          {busy ? "Создаю…" : "Создать партию"}
        </button>
        {!user && status === "ready" && (
          <p className="lobby__note">Вы не вошли: мы создадим вам гостевой аккаунт. Позже его можно привязать к Google.</p>
        )}
      </section>

      <p className="lobby__soon">Подбор соперника по рейтингу и игра на рейтинг появятся позже.</p>
    </div>
  );
}

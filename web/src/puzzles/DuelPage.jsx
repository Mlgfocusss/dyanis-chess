import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { fetchRatings, seekLimits } from "../online/api.js";
import { useSettings } from "../settings/SettingsContext.jsx";
import { ratingText } from "../online/rating.js";
import useSeek from "../online/useSeek.js";
import Icon from "../ui/Icon.jsx";
import DuelRoom from "./DuelRoom.jsx";
import PuzzleTabs from "./PuzzleTabs.jsx";
import "../online/online.css";
import "./puzzles.css";

// "#puzzles/duel" is the lobby, "#puzzles/duel/<id>" a duel that is on.
function duelId() {
  const [page, part, id] = window.location.hash.slice(1).split("/");
  return page === "puzzles" && part === "duel" && /^[0-9a-f]{16}$/.test(id || "") ? id : "";
}

export default function DuelPage() {
  const id = duelId();
  return id ? <DuelRoom key={id} id={id} /> : <DuelLobby />;
}

// Two people, brought together by their rating, solve the same puzzles for three minutes.
function DuelLobby() {
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();
  const [mine, setMine] = useState(null);
  const [error, setError] = useState("");
  const seek = useSeek((st) => {
    window.location.hash = `puzzles/duel/${st.id}`;
  });

  useEffect(() => {
    if (status !== "ready" || !user) return;
    fetchRatings().then((all) => setMine(all.find((r) => r.category === "duel") || null), () => {});
  }, [status, user]);

  async function find() {
    setError("");
    try {
      if (!user) await guest();
      seek.start({ kind: "duel", ...seekLimits(settings) });
    } catch (e) {
      setError(e.message);
    }
  }

  const searching = seek.status !== null;
  if (status === "unavailable") {
    return (
      <div className="page lobby">
        <div className="banner banner--error">Дуэль недоступна: сервер запущен без базы данных.</div>
      </div>
    );
  }
  return (
    <div className="page lobby duel">
      <header className="page__head">
        <h1 className="page__title">Тактическая дуэль</h1>
        <p className="page__sub">Вы и живой соперник решаете одни и те же задачи наперегонки.</p>
      </header>
      <div className="duel__tabs">
        <PuzzleTabs mode="duel" />
      </div>
      {(error || seek.error) && <div className="banner banner--error">{error || seek.error}</div>}

      <section className="card lobby__card">
        <h2 className="lobby__h">Ваш рейтинг в дуэлях</h2>
        <p className="puzzles__big">{mine && mine.games ? ratingText(mine.rating, mine.provisional) : "1200?"}</p>
        <p className="room__meta">
          {mine && mine.games ? `Дуэлей: ${mine.games}, побед ${mine.wins}, ничьих ${mine.draws}, поражений ${mine.losses}.` : "Вы ещё не играли дуэли."} Рейтинг считается по системе Glicko-2, как в рейтинговых партиях.
        </p>

        {searching ? (
          <div className="lobby__search" role="status" aria-live="polite">
            <span className="lobby__spin" aria-hidden="true" />
            <div>
              <p className="lobby__searching">Ищем соперника…</p>
              <p className="lobby__note">
                {seek.status.waited || 0} с{seek.status.window ? ` · разница в рейтинге до ${Math.round(seek.status.window)}` : ""}
                {seek.status.count > 1 ? ` · в очереди: ${seek.status.count}` : ""}
              </p>
            </div>
            <button type="button" className="btn" onClick={seek.cancel}>
              Отмена
            </button>
          </div>
        ) : (
          <button type="button" className="btn btn--primary lobby__go" onClick={find} disabled={status === "loading"}>
            <Icon name="duel" size={18} /> Найти соперника
          </button>
        )}
      </section>

      <section className="card lobby__card">
        <h2 className="lobby__h">Правила</h2>
        <ul className="duel__rules">
          <li>Дуэль идёт 3 минуты. Перед началом 5 секунд на подготовку.</li>
          <li>Задачи у обоих одни и те же и идут от лёгких к сложным. Каждый решает в своём темпе.</li>
          <li>Решённая задача даёт очко. Ошибка не заканчивает дуэль: задача пропускается, а ошибки считаются при равенстве очков.</li>
          <li>Побеждает тот, у кого больше очков. Очки соперника видно в ходе дуэли.</li>
        </ul>
      </section>
    </div>
  );
}

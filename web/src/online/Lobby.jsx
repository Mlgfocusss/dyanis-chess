import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { createGame, ensureNoGameRunning, fetchRatings, seekLimits } from "./api.js";
import { TIME_CONTROLS, tcToRequest } from "./clock.js";
import { CATEGORY_NAMES, categoryOf, ratingText, searchLine } from "./rating.js";
import useSeek from "./useSeek.js";
import "./online.css";

const COLORS = [
  ["white", "Белые"],
  ["random", "Случайно"],
  ["black", "Чёрные"],
];

const KINDS = [
  ["rated", "Рейтинговая"],
  ["casual", "Обычная"],
];

const label = ([m, inc]) => (inc ? `${m} + ${inc}` : `${m} мин`);

// "Онлайн": find a person to play (rated or casual), or make a game and send a friend the link.
export default function Lobby() {
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();
  const [tc, setTc] = useState([5, 0]);
  const [color, setColor] = useState("random");
  const [kind, setKind] = useState("rated"); // for the quick game
  const [friendKind, setFriendKind] = useState("casual"); // for the game with a friend
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [mine, setMine] = useState([]); // my ratings
  const [searching, setSearching] = useState(null); // the time control being searched

  const seek = useSeek((st) => {
    window.location.hash = `online/${st.id}`;
  });

  useEffect(() => {
    if (status !== "ready" || !user) return;
    fetchRatings().then(setMine, () => {});
  }, [status, user]);
  const ratingOf = (cat) => mine.find((r) => r.category === cat);

  async function create() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest(); // one click: the game needs a person, a guest will do
      await ensureNoGameRunning(settings);
      const id = await createGame({ color, ...tcToRequest(tc), rated: friendKind === "rated" });
      window.location.hash = `online/${id}`;
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  async function find(o) {
    setError("");
    try {
      if (!user) await guest();
      await ensureNoGameRunning(settings);
      setSearching(o);
      const { timeMs, incMs } = tcToRequest(o);
      seek.start({ kind: "chess", baseMs: timeMs, incMs, rated: kind === "rated", ...seekLimits(settings) });
    } catch (e) {
      setError(e.message);
    }
  }

  const same = (a) => a[0] === tc[0] && a[1] === tc[1];
  const unavailable = status === "unavailable";
  const searchingNow = seek.status !== null;

  return (
    <div className="page lobby">
      <header className="page__head">
        <h1 className="page__title">Играть онлайн</h1>
        <p className="page__sub">Выберите время, и сайт подберёт соперника с близким рейтингом. Или создайте партию и отправьте другу ссылку.</p>
      </header>

      {unavailable && <div className="banner banner--error">Онлайн-игра недоступна: сервер запущен без базы данных.</div>}
      {(error || seek.error) && <div className="banner banner--error">{error || seek.error}</div>}

      <section className="card lobby__card">
        <h2 className="lobby__h">Быстрая игра</h2>
        <div className="lobby__ratings" aria-label="Ваши рейтинги">
          {["bullet", "blitz", "rapid"].map((c) => {
            const r = ratingOf(c);
            return (
              <span key={c} className="lobby__rating">
                {CATEGORY_NAMES[c]} <b>{r ? ratingText(r.rating, r.provisional) : "—"}</b>
              </span>
            );
          })}
        </div>
        <Segmented label="Вид игры" value={kind} onChange={setKind} options={KINDS} />

        {searchingNow ? (
          <div className="lobby__search" role="status" aria-live="polite">
            <span className="lobby__spin" aria-hidden="true" />
            <div>
              <p className="lobby__searching">Ищем соперника: {searching ? label(searching) : ""}</p>
              <p className="lobby__note">{searchLine(seek.status)}</p>
            </div>
            <button type="button" className="btn" onClick={seek.cancel}>
              Отмена
            </button>
          </div>
        ) : (
          TIME_CONTROLS.map((g) => (
            <div key={g.group} className="lobby__group">
              <span className="lobby__gname">{g.group}</span>
              <div className="lobby__opts">
                {g.options.map((o) => (
                  <button key={o.join("+")} type="button" className="chip chip--big" disabled={unavailable || status === "loading"} onClick={() => find(o)}>
                    {label(o)}
                  </button>
                ))}
              </div>
            </div>
          ))
        )}
        {!searchingNow && (
          <p className="lobby__note">
            {kind === "rated" ? "Результат изменит рейтинг обоих игроков (Glicko-2): по времени партия попадёт в «Пулю», «Блиц» или «Рапид». " : "Обычная партия рейтинг не меняет. "}
            Соперник подбирается по близкому рейтингу, чем дольше ждёте, тем шире разница.
          </p>
        )}
        {!user && status === "ready" && !searchingNow && (
          <p className="lobby__note">Вы не вошли: мы создадим вам гостевой аккаунт. Позже его можно привязать к Google.</p>
        )}
      </section>

      <section className="card lobby__card">
        <h2 className="lobby__h">Партия с другом по ссылке</h2>
        {TIME_CONTROLS.map((g) => (
          <div key={g.group} className="lobby__group">
            <span className="lobby__gname">{g.group}</span>
            <div className="lobby__opts">
              {g.options.map((o) => (
                <button key={o.join("+")} type="button" className={`chip${same(o) ? " chip--on" : ""}`} aria-pressed={same(o)} onClick={() => setTc(o)}>
                  {label(o)}
                </button>
              ))}
            </div>
          </div>
        ))}

        <h2 className="lobby__h">Ваш цвет</h2>
        <Segmented label="Цвет" value={color} onChange={setColor} options={COLORS} />
        <h2 className="lobby__h">Вид игры</h2>
        <Segmented label="Вид игры" value={friendKind} onChange={setFriendKind} options={KINDS} />
        {friendKind === "rated" && (
          <p className="lobby__note">
            Рейтинговая партия: {CATEGORY_NAMES[categoryOf(tc[0], tc[1])]}. Она изменит рейтинг обоих, когда закончится.
          </p>
        )}

        <button type="button" className="btn btn--primary lobby__go" onClick={create} disabled={busy || unavailable || status === "loading" || searchingNow}>
          <Icon name="online" size={18} /> {busy ? "Создаю…" : "Создать партию"}
        </button>
      </section>
    </div>
  );
}

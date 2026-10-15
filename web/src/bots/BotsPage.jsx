import { useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import GameRoom from "../online/GameRoom.jsx";
import { TIME_CONTROLS, tcToRequest } from "../online/clock.js";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import BotAvatar from "./BotAvatar.jsx";
import { createBotGame, useBots } from "./api.js";
import "./bots.css";

const COLORS = [
  ["white", "Белыми"],
  ["random", "Случайно"],
  ["black", "Чёрными"],
];
const NO_CLOCK = "none";
const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

// "#bots" is the choice of an opponent, "#bots/<id>" a game against one.
function idFromHash() {
  const [page, id] = window.location.hash.slice(1).split("/");
  return page === "bots" ? id || "" : "";
}

export default function BotsPage() {
  const [id, setId] = useState(idFromHash);
  useEffect(() => {
    const onHash = () => setId(idFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return id ? <GameRoom key={id} id={id} backHash="bots" backLabel="К ботам" /> : <BotPicker />;
}

function Row({ bot, selected, progress, onSelect }) {
  const beaten = progress?.beaten;
  return (
    <button type="button" className={`botrow${selected ? " botrow--on" : ""}`} onClick={() => onSelect(bot.id)} aria-pressed={selected}>
      <BotAvatar bot={bot} size={44} />
      <span className="botrow__main">
        <span className="botrow__name">{bot.name}</span>
        <span className="botrow__elo">{bot.elo}</span>
      </span>
      {beaten ? (
        <span className="botrow__done" title="Бот побеждён">
          <Icon name="check" size={16} /> побеждён
        </span>
      ) : (
        <span className="botrow__pts" title="Очки за первую победу">
          +{bot.points}
        </span>
      )}
    </button>
  );
}

function BotPicker() {
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();
  const { data, error: loadError } = useBots(user?.id);
  const [picked, setPicked] = useState("dionis");
  const [color, setColor] = useState("random");
  const [clock, setClock] = useState(NO_CLOCK);
  const [open, setOpen] = useState({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const bots = data?.bots || [];
  const bot = bots.find((b) => b.id === picked) || bots[0];
  const progress = data?.progress || {};
  const total = bots.length;
  const beaten = bots.filter((b) => progress[b.id]?.beaten).length;

  // the board in the preview is seen from the side the person will play
  const orientation = color === "black" ? "black" : "white";

  const groups = useMemo(
    () => (data?.categories || []).map((c) => ({ ...c, bots: bots.filter((b) => b.category === c.id) })).filter((g) => g.bots.length),
    [data] // eslint-disable-line react-hooks/exhaustive-deps
  );

  async function play() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest();
      const tc = clock === NO_CLOCK ? { timeMs: 0, incMs: 0 } : tcToRequest(clock.split("+").map(Number));
      const gameId = await createBotGame({ bot: bot.id, color, ...tc });
      window.location.hash = `bots/${gameId}`;
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  const bp = bot ? progress[bot.id] : null;
  return (
    <div className="botspage">
      <div className="botspage__grid">
        <div className="botspage__board">
          <div className="botspage__who">
            {bot && <BotAvatar bot={bot} size={36} />}
            <span className="botspage__name">
              {bot?.name} <span className="botspage__elo">({bot?.elo})</span>
            </span>
          </div>
          <ThemedBoard id="BotsPreview" position={START} orientation={orientation} arePiecesDraggable={false} settings={settings} />
          <div className="botspage__who">
            <span className="botspage__name">{user?.name || "Вы"}</span>
          </div>
        </div>

        <aside className="card botspanel">
          <header className="botspanel__head">
            <h1>Боты</h1>
            {user && data && (
              <span className="botspanel__score" title="Очки за первые победы над ботами">
                <Icon name="trophy" size={16} /> {data.points} очков · {beaten} из {total}
              </span>
            )}
          </header>

          {bot && (
            <section className="botcard">
              <BotAvatar bot={bot} size={72} />
              <div className="botcard__text">
                <p className="botcard__bubble">{bot.greeting}</p>
                <p className="botcard__title">
                  <b>{bot.name}</b> <span>{bot.elo}</span>
                </p>
                <p className="botcard__desc">{bot.desc}</p>
                <p className="botcard__reward">
                  {bp?.beaten ? (
                    <>
                      <Icon name="check" size={14} /> Побеждён, очки получены (+{bp.points}). Побед: {bp.wins}, ничьих: {bp.draws}, поражений: {bp.losses}
                    </>
                  ) : (
                    <>
                      <Icon name="trophy" size={14} /> Первая победа: +{bot.points} очков
                      {bp ? `. Ничьих: ${bp.draws}, поражений: ${bp.losses}` : ""}
                    </>
                  )}
                </p>
              </div>
            </section>
          )}

          <div className="botlist" role="list">
            {loadError && <div className="banner banner--error">{loadError}</div>}
            {groups.map((g) => {
              const collapsed = open[g.id] === false;
              const done = g.bots.filter((b) => progress[b.id]?.beaten).length;
              return (
                <div key={g.id} className="botgroup">
                  <button
                    type="button"
                    className="botgroup__head"
                    aria-expanded={!collapsed}
                    onClick={() => setOpen((o) => ({ ...o, [g.id]: collapsed }))}
                  >
                    <span>{g.name}</span>
                    <span className="botgroup__count">
                      {done}/{g.bots.length}
                    </span>
                    <Icon name="chevron" size={16} className={`botgroup__chev${collapsed ? "" : " botgroup__chev--open"}`} />
                  </button>
                  {!collapsed &&
                    g.bots.map((b) => (
                      <Row key={b.id} bot={b} selected={b.id === bot?.id} progress={progress[b.id]} onSelect={setPicked} />
                    ))}
                </div>
              );
            })}
          </div>

          <div className="botopts">
            <Segmented label="Цвет" value={color} onChange={setColor} options={COLORS} />
            <label className="botopts__clock">
              <Icon name="clock" size={16} />
              <select className="field" value={clock} onChange={(e) => setClock(e.target.value)} aria-label="Время на партию">
                <option value={NO_CLOCK}>Без часов</option>
                {TIME_CONTROLS.map((g) => (
                  <optgroup key={g.group} label={g.group}>
                    {g.options.map((o) => (
                      <option key={o.join("+")} value={o.join("+")}>
                        {o[1] ? `${o[0]} + ${o[1]}` : `${o[0]} мин`}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </label>
          </div>
          {error && <div className="banner banner--error">{error}</div>}
          <button type="button" className="btn btn--primary botspanel__play" onClick={play} disabled={busy || !bot || status === "unavailable"}>
            {busy ? "Начинаем…" : "Играть"}
          </button>
          {status === "unavailable" && <p className="botspanel__note">Игра с ботами недоступна: сервер запущен без базы данных.</p>}
          <p className="botspanel__note">
            Рейтинги ботов ориентировочные, по шкале нашего движка: это не рейтинг FIDE и не рейтинг chess.com.
          </p>
        </aside>
      </div>
    </div>
  );
}

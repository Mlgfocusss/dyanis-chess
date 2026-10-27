import { useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import GameRoom from "../online/GameRoom.jsx";
import { TIME_CONTROLS, tcToRequest } from "../online/clock.js";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import BotAvatar from "./BotAvatar.jsx";
import { botCategoryName, botText } from "./botText.js";
import { ensureNoGameRunning } from "../online/api.js";
import { createBotGame, useBots } from "./api.js";
import { tOpts, useT } from "../i18n/index.js";
import "./bots.css";

const COLORS = [
  ["white", "onlineGames.8"],
  ["random", "bots.random"],
  ["black", "onlineGames.9"],
];
const NO_CLOCK = "none";
const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

// "#bots" is the choice of an opponent, "#bots/<id>" a game against one.
function idFromHash() {
  const [page, id] = window.location.hash.slice(1).split("/");
  return page === "bots" ? id || "" : "";
}

export default function BotsPage() {
  const t = useT();
  const [id, setId] = useState(idFromHash);
  useEffect(() => {
    const onHash = () => setId(idFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return id ? <GameRoom key={id} id={id} backHash="bots" backLabel={t("botsPage.4")} /> : <BotPicker />;
}

function Row({ bot, selected, progress, onSelect }) {
  const t = useT();
  const beaten = progress?.beaten;
  return (
    <button type="button" className={`botrow${selected ? " botrow--on" : ""}`} onClick={() => onSelect(bot.id)} aria-pressed={selected}>
      <BotAvatar bot={bot} size={44} />
      <span className="botrow__main">
        <span className="botrow__name">{botText(t, bot.id, "name", bot.name)}</span>
        <span className="botrow__elo">{bot.elo}</span>
      </span>
      {beaten ? (
        <span className="botrow__done" title={t("botsPage.5")}>
          <Icon name="check" size={16} /> {t("botsPage.6")}
        </span>
      ) : (
        <span className="botrow__pts" title={t("botsPage.7")}>
          +{bot.points}
        </span>
      )}
    </button>
  );
}

function BotPicker() {
  const t = useT();
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
      await ensureNoGameRunning(settings);
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
              {botText(t, bot?.id, "name", bot?.name)} <span className="botspage__elo">({bot?.elo})</span>
            </span>
          </div>
          <ThemedBoard id="BotsPreview" position={START} orientation={orientation} arePiecesDraggable={false} settings={settings} />
          <div className="botspage__who">
            <span className="botspage__name">{user?.name || t("botsPage.8")}</span>
          </div>
        </div>

        <aside className="card botspanel">
          <header className="botspanel__head">
            <h1>{t("avatar.bots")}</h1>
            {user && data && (
              <span className="botspanel__score" title={t("botsPage.10")}>
                <Icon name="trophy" size={16} /> {data.points} {t("botsPage.11")} {beaten} {t("botsPage.12")} {total}
              </span>
            )}
          </header>

          {bot && (
            <section className="botcard">
              <BotAvatar bot={bot} size={72} />
              <div className="botcard__text">
                <p className="botcard__bubble">{botText(t, bot.id, "greeting", bot.greeting)}</p>
                <p className="botcard__title">
                  <b>{botText(t, bot.id, "name", bot.name)}</b> <span>{bot.elo}</span>
                </p>
                <p className="botcard__desc">{botText(t, bot.id, "desc", bot.desc)}</p>
                <p className="botcard__reward">
                  {bp?.beaten ? (
                    <>
                      <Icon name="check" size={14} /> {t("botsPage.13")}{bp.points}{t("botsPage.14")} {bp.wins}{t("botsPage.15")} {bp.draws}{t("botsPage.16")} {bp.losses}
                    </>
                  ) : (
                    <>
                      <Icon name="trophy" size={14} /> {t("botsPage.17")}{bot.points} {t("botsPage.18")}
                      {bp ? t("botsPage.19", { draws: bp.draws, losses: bp.losses }) : ""}
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
                    <span>{botCategoryName(t, g)}</span>
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
            <Segmented label={t("online.colour")} value={color} onChange={setColor} options={tOpts(t, COLORS)} />
            <label className="botopts__clock">
              <Icon name="clock" size={16} />
              <select className="field" value={clock} onChange={(e) => setClock(e.target.value)} aria-label={t("botsPage.21")}>
                <option value={NO_CLOCK}>{t("botsPage.22")}</option>
                {TIME_CONTROLS.map((g) => (
                  <optgroup key={g.group} label={t(g.group)}>
                    {g.options.map((o) => (
                      <option key={o.join("+")} value={o.join("+")}>
                        {o[1] ? `${o[0]} + ${o[1]}` : t("botsPage.23", { p: o[0] })}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </label>
          </div>
          {error && <div className="banner banner--error">{error}</div>}
          <button type="button" className="btn btn--primary botspanel__play" onClick={play} disabled={busy || !bot || status === "unavailable"}>
            {busy ? t("botsPage.24") : t("botsPage.play")}
          </button>
          {status === "unavailable" && <p className="botspanel__note">{t("botsPage.26")}</p>}
          <p className="botspanel__note">
            {t("botsPage.27")}
          </p>
        </aside>
      </div>
    </div>
  );
}

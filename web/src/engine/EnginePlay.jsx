import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import GameRoom from "../online/GameRoom.jsx";
import { TIME_CONTROLS, tcToRequest } from "../online/clock.js";
import { ensureNoGameRunning } from "../online/api.js";
import BotAvatar from "../bots/BotAvatar.jsx";
import { createBotGame, useBots } from "../bots/api.js";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { tOpts, useT } from "../i18n/index.js";
import "../bots/bots.css";

// "Play the engine" is a game against the strongest bot ("max": the whole engine with the network, on the
// server, see backend/internal/bots) in the usual game room. #play is the choice of colour and clock,
// #play/<id> the game; #play/local is the older page with the engine compiled to WebAssembly (no network,
// weaker, works without a server: App.jsx).
const ENGINE_BOT = "max";
const COLORS = [
  ["white", "onlineGames.8"],
  ["random", "bots.random"],
  ["black", "onlineGames.9"],
];
const NO_CLOCK = "none";
const START = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";

function idFromHash() {
  const [page, id] = window.location.hash.slice(1).split("/");
  return page === "play" && id !== "local" ? id || "" : "";
}

export default function EnginePlay() {
  const t = useT();
  const [id, setId] = useState(idFromHash);
  useEffect(() => {
    const onHash = () => setId(idFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return id ? <GameRoom key={id} id={id} backHash="play" backLabel={t("engineVs.back")} /> : <Picker />;
}

function Picker() {
  const t = useT();
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();
  const { data } = useBots(user?.id);
  const [color, setColor] = useState("random");
  const [clock, setClock] = useState(NO_CLOCK);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const bot = (data?.bots || []).find((b) => b.id === ENGINE_BOT) || { id: ENGINE_BOT, name: "Dyanis" };

  async function play() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest();
      await ensureNoGameRunning(settings);
      const tc = clock === NO_CLOCK ? { timeMs: 0, incMs: 0 } : tcToRequest(clock.split("+").map(Number));
      const gameId = await createBotGame({ bot: ENGINE_BOT, color, ...tc });
      window.location.hash = `play/${gameId}`;
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <div className="botspage">
      <div className="botspage__grid">
        <div className="botspage__board">
          <div className="botspage__who">
            <BotAvatar bot={bot} size={36} />
            <span className="botspage__name">{t("engineVs.name")}</span>
          </div>
          <ThemedBoard id="EnginePreview" position={START} orientation={color === "black" ? "black" : "white"} arePiecesDraggable={false} settings={settings} />
          <div className="botspage__who">
            <span className="botspage__name">{user?.name || t("botsPage.8")}</span>
          </div>
        </div>

        <aside className="card botspanel">
          <header className="botspanel__head">
            <h1>{t("engineVs.title")}</h1>
          </header>
          <section className="botcard">
            <BotAvatar bot={bot} size={72} />
            <div className="botcard__text">
              <p className="botcard__title">
                <b>{t("engineVs.name")}</b> <span>≈ 3000</span>
              </p>
              <p className="botcard__desc">{t("engineVs.desc")}</p>
            </div>
          </section>
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
          <button type="button" className="btn btn--primary botspanel__play" onClick={play} disabled={busy || status === "unavailable"}>
            {busy ? t("botsPage.24") : t("botsPage.play")}
          </button>
          {status === "unavailable" && <p className="botspanel__note">{t("engineVs.noServer")}</p>}
          <p className="botspanel__note">{t("engineVs.note")}</p>
          <p className="botspanel__note">
            <a href="#play/local">{t("engineVs.local")}</a>
          </p>
        </aside>
      </div>
    </div>
  );
}

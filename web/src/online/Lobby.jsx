import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { createGame, ensureNoGameRunning, fetchRatings, seekLimits } from "./api.js";
import { TIME_CONTROLS, tcToRequest } from "./clock.js";
import { CORR_DAYS, daysText } from "../community/logic.js";
import { categoryName, categoryOf, ratingText, searchLine } from "./rating.js";
import useSeek from "./useSeek.js";
import { tOpts, useT, tr } from "../i18n/index.js";
import "./online.css";

const COLORS = [
  ["white", "online.white"],
  ["random", "bots.random"],
  ["black", "online.black"],
];

const KINDS = [
  ["rated", "online.rated"],
  ["casual", "online.casual"],
];

const label = ([m, inc]) => (inc ? `${m} + ${inc}` : tr("lobby.6", { m }));

// "Онлайн": find a person to play (rated or casual), or make a game and send a friend the link.
export default function Lobby() {
  const t = useT();
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
  const [corrDays, setCorrDays] = useState(settings.corrDays || 3); // days per move of a correspondence game
  const [corrColor, setCorrColor] = useState("random");

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

  async function createCorr() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest();
      const id = await createGame({ color: corrColor, timeMs: 0, incMs: 0, days: corrDays });
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
        <h1 className="page__title">{t("home.playOnline")}</h1>
        <p className="page__sub">{t("lobby.8")}</p>
      </header>

      {unavailable && <div className="banner banner--error">{t("lobby.9")}</div>}
      {(error || seek.error) && <div className="banner banner--error">{error || seek.error}</div>}

      <section className="card lobby__card">
        <h2 className="lobby__h">{t("lobby.10")}</h2>
        <div className="lobby__ratings" aria-label={t("lobby.11")}>
          {["bullet", "blitz", "rapid"].map((c) => {
            const r = ratingOf(c);
            return (
              <span key={c} className="lobby__rating">
                {categoryName(c)} <b>{r ? ratingText(r.rating, r.provisional) : "—"}</b>
              </span>
            );
          })}
        </div>
        <Segmented label={t("lobby.12")} value={kind} onChange={setKind} options={tOpts(t, KINDS)} />

        {searchingNow ? (
          <div className="lobby__search" role="status" aria-live="polite">
            <span className="lobby__spin" aria-hidden="true" />
            <div>
              <p className="lobby__searching">{t("lobby.13")} {searching ? label(searching) : ""}</p>
              <p className="lobby__note">{searchLine(seek.status)}</p>
            </div>
            <button type="button" className="btn" onClick={seek.cancel}>
              {t("common.cancel")}
            </button>
          </div>
        ) : (
          TIME_CONTROLS.map((g) => (
            <div key={t(g.group)} className="lobby__group">
              <span className="lobby__gname">{t(g.group)}</span>
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
            {kind === "rated" ? t("lobby.15") : t("lobby.16")}
            {t("lobby.17")}
          </p>
        )}
        {!user && status === "ready" && !searchingNow && (
          <p className="lobby__note">{t("lobby.18")}</p>
        )}
      </section>

      <section className="card lobby__card">
        <h2 className="lobby__h">{t("lobby.19")}</h2>
        {TIME_CONTROLS.map((g) => (
          <div key={t(g.group)} className="lobby__group">
            <span className="lobby__gname">{t(g.group)}</span>
            <div className="lobby__opts">
              {g.options.map((o) => (
                <button key={o.join("+")} type="button" className={`chip${same(o) ? " chip--on" : ""}`} aria-pressed={same(o)} onClick={() => setTc(o)}>
                  {label(o)}
                </button>
              ))}
            </div>
          </div>
        ))}

        <h2 className="lobby__h">{t("lobby.20")}</h2>
        <Segmented label={t("online.colour")} value={color} onChange={setColor} options={tOpts(t, COLORS)} />
        <h2 className="lobby__h">{t("lobby.12")}</h2>
        <Segmented label={t("lobby.12")} value={friendKind} onChange={setFriendKind} options={tOpts(t, KINDS)} />
        {friendKind === "rated" && (
          <p className="lobby__note">
            {t("lobby.22")} {categoryName(categoryOf(tc[0], tc[1]))}{t("lobby.23")}
          </p>
        )}

        <button type="button" className="btn btn--primary lobby__go" onClick={create} disabled={busy || unavailable || status === "loading" || searchingNow}>
          <Icon name="online" size={18} /> {busy ? t("lobby.24") : t("lobby.25")}
        </button>
      </section>

      <section className="card lobby__card" id="corr">
        <h2 className="lobby__h">{t("lobby.26")}</h2>
        <p className="lobby__note">
          {t("lobby.27")} <a href="#community/friends">{t("lobby.28")}</a>{t("lobby.29")} <a href="#community/correspondence">{t("lobby.30")}</a>.
        </p>
        <div className="lobby__group">
          <span className="lobby__gname">{t("lobby.31")}</span>
          <div className="lobby__opts">
            {CORR_DAYS.map((d) => (
              <button key={d} type="button" className={`chip${corrDays === d ? " chip--on" : ""}`} aria-pressed={corrDays === d} onClick={() => setCorrDays(d)}>
                {daysText(d)}
              </button>
            ))}
          </div>
        </div>
        <Segmented label={t("online.colour")} value={corrColor} onChange={setCorrColor} options={tOpts(t, COLORS)} />
        <button type="button" className="btn btn--primary lobby__go" onClick={createCorr} disabled={busy || unavailable || status === "loading" || searchingNow}>
          <Icon name="clock" size={18} /> {busy ? t("lobby.24") : t("lobby.32")}
        </button>
      </section>
    </div>
  );
}

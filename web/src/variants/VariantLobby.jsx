import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { TIME_CONTROLS, tcToRequest } from "../online/clock.js";
import useSeek from "../online/useSeek.js";
import { SEEK_API, createGame, ensureNoGameRunning } from "./api.js";
import { tx, tOpts, useT, tr } from "../i18n/index.js";
import "../online/online.css";
import "./variants.css";

const COLORS = [
  ["white", "online.white"],
  ["random", "bots.random"],
  ["black", "online.black"],
];

const label = ([m, inc]) => (m === 0 ? tr("clock.2") : inc ? `${m} + ${inc}` : tr("lobby.6", { m }));

// The lobby of a variant: its rules, the choices it has (a def.lobby.Options form), a quick game with a
// person who wants the same, and a game with a friend by a link. mode ("quick" or "link") puts that way first.
export default function VariantLobby({ def, mode = "" }) {
  const t = useT();
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();
  const [tc, setTc] = useState([5, 0]);
  const [color, setColor] = useState("random");
  const [options, setOptions] = useState(() => def.lobby?.defaults ?? undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [searching, setSearching] = useState(null);

  const seek = useSeek((st) => {
    window.location.hash = `variants/game/${st.id}`;
  }, SEEK_API);

  const Options = def.lobby?.Options;
  const optionsCard = Options && (
    <section key="options" className="card lobby__card">
      <h2 className="lobby__h">{t("vlobby.6")}</h2>
      <Options value={options} onChange={setOptions} />
    </section>
  );

  async function create() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest(); // one click: the game needs a person, a guest will do
      await ensureNoGameRunning(settings);
      const id = await createGame({ variant: def.id, color, ...tcToRequest(tc), options });
      window.location.hash = `variants/game/${id}`;
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
      seek.start({ variant: def.id, baseMs: timeMs, incMs, ...(options ? { options } : {}) });
    } catch (e) {
      setError(e.message);
    }
  }

  const same = (a) => a[0] === tc[0] && a[1] === tc[1];
  const unavailable = status === "unavailable";
  const searchingNow = seek.status !== null;
  const four = def.seats === 4; // Bughouse: four people, no quick game, only a link
  const quickFirst = mode !== "link";

  const quickCard = (
    <section key="quick" className="card lobby__card" id="quick">
      <h2 className="lobby__h">{t("menu.quick")}</h2>
      {searchingNow ? (
        <div className="lobby__search" role="status" aria-live="polite">
          <span className="lobby__spin" aria-hidden="true" />
          <div>
            <p className="lobby__searching">{t("lobby.13")} {searching ? label(searching) : ""}</p>
            <p className="lobby__note">
              {seek.status?.count > 1 ? t("vlobby.9", { count: seek.status.count }) : ""}
              {t("vlobby.10")} {seek.status?.waited || 0} {t("vlobby.11")}
            </p>
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
          {t("vlobby.13")}
        </p>
      )}
      {!user && status === "ready" && !searchingNow && (
        <p className="lobby__note">{t("lobby.18")}</p>
      )}
    </section>
  );

  const linkCard = (
    <section key="link" className="card lobby__card" id="link">
      <h2 className="lobby__h">{t("lobby.19")}</h2>
      {[...TIME_CONTROLS, { group: t("clock.noClock"), options: [[0, 0]] }].map((g) => (
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
      <h2 className="lobby__h">{four ? t("vlobby.17") : t("lobby.20")}</h2>
      <Segmented label={t("online.colour")} value={color} onChange={setColor} options={tOpts(t, COLORS)} />
      <button type="button" className="btn btn--primary lobby__go" onClick={create} disabled={busy || unavailable || status === "loading" || searchingNow}>
        <Icon name="online" size={18} /> {busy ? t("lobby.24") : t("lobby.25")}
      </button>
      <p className="lobby__note">
        {four
          ? t("vlobby.22")
          : t("vlobby.23")}
      </p>
    </section>
  );

  return (
    <div className="page lobby vlobby">
      <header className="page__head vlobby__head">
        <a className="vlobby__back" href="#variants">
          <Icon name="back" size={16} /> {t("vlobby.all")}
        </a>
        <h1 className="page__title">{tx(def.name)}</h1>
        <p className="page__sub">{tx(def.short)}</p>
      </header>

      {unavailable && <div className="banner banner--error">{t("lobby.9")}</div>}
      {(error || seek.error) && <div className="banner banner--error">{error || seek.error}</div>}

      {def.rules.length > 0 && (
        <section className="card vlobby__rules">
          <h2 className="lobby__h">{t("vlobby.26")}</h2>
          <ul>
            {def.rules.map((r, i) => (
              <li key={i}>{tx(r)}</li>
            ))}
          </ul>
        </section>
      )}

      {optionsCard}
      {four ? linkCard : quickFirst ? [quickCard, linkCard] : [linkCard, quickCard]}
    </div>
  );
}

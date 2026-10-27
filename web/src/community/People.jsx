import { useState } from "react";
import Avatar from "../auth/Avatar.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { TIME_CONTROLS, tcToRequest } from "../online/clock.js";
import { sendChallenge } from "./api.js";
import { useCommunity } from "./CommunityContext.jsx";
import { CORR_DAYS, daysText } from "./logic.js";
import { tOpts, useT, tr } from "../i18n/index.js";

// A person as a link to their page: picture, name, a green dot when they are on the site.
export function PersonCell({ p, size = 36, children }) {
  const t = useT();
  return (
    <span className="cperson">
      <a className="cperson__link" href={`#user/${p.id}`} title={t("people.1", { name: p.name })}>
        <span className="cperson__pic">
          <Avatar user={p} size={size} />
          {p.online && <span className="cperson__dot" title={t("people.2")} />}
        </span>
        <span className="cperson__name">{p.name}</span>
      </a>
      {children}
    </span>
  );
}

const COLORS = [
  ["white", "onlineGames.8"],
  ["random", "bots.random"],
  ["black", "onlineGames.9"],
];
const KINDS = [
  ["clock", "people.clock"],
  ["corr", "people.corr"],
];
const RATED = [
  ["casual", "online.casual"],
  ["rated", "online.rated"],
];
const label = ([m, inc]) => (inc ? `${m} + ${inc}` : t("lobby.6", { m }));

// The form of a challenge to a game. `person` is who is challenged; it closes itself when sent.
export function ChallengeForm({ person, onClose, onSent }) {
  const t = useT();
  const { settings } = useSettings();
  const { refresh } = useCommunity();
  const [kind, setKind] = useState("clock");
  const [tc, setTc] = useState([5, 0]);
  const [days, setDays] = useState(settings.corrDays || 3);
  const [color, setColor] = useState("random");
  const [rated, setRated] = useState("casual");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const same = (a) => a[0] === tc[0] && a[1] === tc[1];

  async function send() {
    setBusy(true);
    setError("");
    try {
      const body =
        kind === "corr"
          ? { to: person.id, color, baseMs: 0, incMs: 0, days, rated: false }
          : { to: person.id, color, baseMs: tcToRequest(tc).timeMs, incMs: tcToRequest(tc).incMs, days: 0, rated: rated === "rated" };
      await sendChallenge(body);
      refresh();
      onSent?.();
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <div className="cform card">
      <header className="cform__head">
        <h3>{t("friendsTab.16")} {person.name} {t("people.12")}</h3>
        <button type="button" className="btn btn--ghost" onClick={onClose} aria-label={t("people.13")}>
          <Icon name="close" size={16} />
        </button>
      </header>
      <Segmented label={t("people.14")} value={kind} onChange={setKind} options={tOpts(t, KINDS)} />
      {kind === "clock" ? (
        <>
          {TIME_CONTROLS.map((g) => (
            <div key={t(g.group)} className="cform__group">
              <span className="cform__gname">{t(g.group)}</span>
              <div className="cform__opts">
                {g.options.map((o) => (
                  <button key={o.join("+")} type="button" className={`chip${same(o) ? " chip--on" : ""}`} aria-pressed={same(o)} onClick={() => setTc(o)}>
                    {label(o)}
                  </button>
                ))}
              </div>
            </div>
          ))}
          <Segmented label={t("rating.rating")} value={rated} onChange={setRated} options={tOpts(t, RATED)} />
        </>
      ) : (
        <div className="cform__group">
          <span className="cform__gname">{t("lobby.31")}</span>
          <div className="cform__opts">
            {CORR_DAYS.map((d) => (
              <button key={d} type="button" className={`chip${days === d ? " chip--on" : ""}`} aria-pressed={days === d} onClick={() => setDays(d)}>
                {daysText(d)}
              </button>
            ))}
          </div>
          <p className="cform__note">{t("people.17")}</p>
        </div>
      )}
      <div className="cform__group">
        <span className="cform__gname">{t("lobby.20")}</span>
        <Segmented label={t("lobby.20")} value={color} onChange={setColor} options={tOpts(t, COLORS)} />
      </div>
      {error && <div className="banner banner--error">{error}</div>}
      <button type="button" className="btn btn--primary" onClick={send} disabled={busy}>
        <Icon name="send" size={16} /> {busy ? t("people.19") : t("people.20")}
      </button>
    </div>
  );
}

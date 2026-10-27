import { useCallback, useEffect, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { acceptChallenge, cancelChallenge, declineChallenge, fetchChallenges } from "./api.js";
import { useCommunity } from "./CommunityContext.jsx";
import { ago, communityHash, gameText, timeLeft, yourColorText } from "./logic.js";
import { PersonCell } from "./People.jsx";
import { useT } from "../i18n/index.js";

// "Вызовы": what I was asked to play, and what I asked. Accepting opens the game at once.
export default function ChallengesTab() {
  const t = useT();
  const { refresh } = useCommunity();
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(() => {
    fetchChallenges().then(
      (d) => {
        setData(d);
        setError("");
      },
      (e) => setError(e.message),
    );
  }, []);
  useEffect(() => {
    load();
    const t = setInterval(() => !document.hidden && load(), 4000);
    return () => clearInterval(t);
  }, [load]);

  async function accept(c) {
    setBusy(c.id);
    setError("");
    try {
      const gameId = await acceptChallenge(c.id);
      refresh();
      window.location.hash = `online/${gameId}`;
    } catch (e) {
      setError(e.message);
      setBusy("");
      load();
    }
  }
  async function run(fn, c) {
    setBusy(c.id);
    setError("");
    try {
      await fn(c.id);
    } catch (e) {
      setError(e.message);
    }
    setBusy("");
    load();
    refresh();
  }

  const now = Date.now();
  return (
    <div className="ctab">
      {error && <div className="banner banner--error">{error}</div>}

      <section className="card csection">
        <header className="csection__head">
          <h2>{t("challengesTab.1")}{data?.incoming.length ? ` (${data.incoming.length})` : ""}</h2>
        </header>
        {data && data.incoming.length === 0 && (
          <p className="cnote">{t("challengesTab.2")}</p>
        )}
        <ul className="clist">
          {data?.incoming.map((c) => (
            <li key={c.id} className="clist__row clist__row--wrap">
              <PersonCell p={c.person}>
                <span className="cperson__sub">
                  {c.rating ? `${c.prov ? "~" : ""}${c.rating} · ` : ""}
                  {c.friend ? t("challengesTab.3") : t("challengesTab.4")}
                </span>
              </PersonCell>
              <span className="cchallenge">
                <b>{gameText(c)}</b>
                <span>
                  {c.rated ? t("challengesTab.5") : t("challengesTab.6")} · {yourColorText(c.color)} {t("challengesTab.7")} {timeLeft(c.expires, now)}
                </span>
              </span>
              <span className="clist__actions">
                <button type="button" className="btn btn--primary" disabled={busy === c.id} onClick={() => accept(c)}>
                  <Icon name="play" size={16} /> {t("gameRoom.64")}
                </button>
                <button type="button" className="btn" disabled={busy === c.id} onClick={() => run(declineChallenge, c)}>
                  {t("gameRoom.65")}
                </button>
              </span>
            </li>
          ))}
        </ul>
      </section>

      <section className="card csection">
        <header className="csection__head">
          <h2>{t("challengesTab.10")}</h2>
          <a className="btn btn--ghost" href={communityHash("friends")}>
            {t("challengesTab.11")}
          </a>
        </header>
        {data && data.outgoing.length === 0 && <p className="cnote">{t("challengesTab.12")}</p>}
        <ul className="clist">
          {data?.outgoing.map((c) => (
            <li key={c.id} className="clist__row clist__row--wrap">
              <PersonCell p={c.person} />
              <span className="cchallenge">
                <b>{gameText(c)}</b>
                <span>
                  {c.rated ? t("challengesTab.5") : t("challengesTab.6")} · {c.status === "pending" ? t("challengesTab.13", { p: timeLeft(c.expires, now) }) : ago(c.created, now)}
                </span>
              </span>
              <span className="clist__actions">
                {c.status === "pending" && (
                  <button type="button" className="btn btn--ghost" disabled={busy === c.id} onClick={() => run(cancelChallenge, c)}>
                    {t("friendsTab.27")}
                  </button>
                )}
                {c.status === "accepted" && c.gameId && (
                  <a className="btn btn--primary" href={`#online/${c.gameId}`}>
                    <Icon name="play" size={16} /> {t("challengesTab.15")}
                  </a>
                )}
                {c.status === "declined" && <span className="cnote">{t("challengesTab.16")}</span>}
              </span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

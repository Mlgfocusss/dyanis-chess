import { useEffect, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { fetchCorrespondence } from "./api.js";
import { useCommunity } from "./CommunityContext.jsx";
import { daysText, timeLeft, urgent } from "./logic.js";
import { PersonCell } from "./People.jsx";
import { locale, useT } from "../i18n/index.js";

// "Заочные партии": my unfinished games with days per move, the ones that wait for me first.
export default function CorrTab() {
  const t = useT();
  const { summary } = useCommunity();
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    const load = () => fetchCorrespondence().then(
      (l) => {
        setItems(l);
        setError("");
      },
      (e) => setError(e.message),
    );
    load();
    const t = setInterval(() => {
      setNow(Date.now());
      if (!document.hidden) load();
    }, 20000);
    return () => clearInterval(t);
  }, []);

  const sorted = items ? [...items].sort((a, b) => Number(b.yourTurn) - Number(a.yourTurn) || a.due - b.due) : null;
  const vacation = summary?.vacation > now ? summary.vacation : 0;
  return (
    <div className="ctab">
      {error && <div className="banner banner--error">{error}</div>}
      {vacation > 0 && (
        <div className="banner banner--info">
          {t("corrTab.1")} {new Date(vacation).toLocaleDateString(locale(), { day: "numeric", month: "long" })}{t("corrTab.2")}{" "}
          <a href="#settings/play">{t("menu.settings")}</a>
        </div>
      )}
      <section className="card csection">
        <header className="csection__head">
          <h2>{t("menu.correspondence")}{sorted ? ` (${sorted.length})` : ""}</h2>
          <a className="btn btn--primary" href="#online">
            <Icon name="play" size={16} /> {t("corrTab.5")}
          </a>
        </header>
        {sorted && sorted.length === 0 && (
          <p className="cnote">
            {t("corrTab.6")}
          </p>
        )}
        <ul className="clist">
          {sorted?.map((g) => (
            <li key={g.id} className="clist__row clist__row--wrap">
              {g.opponent ? <PersonCell p={{ id: g.opponent.id, name: g.opponent.name, avatarUrl: g.opponent.avatar }} /> : <span className="cperson__name">{t("openings.opponent")}</span>}
              <span className="cchallenge">
                <b>{g.yourTurn ? t("gameRoom.41") : t("gameRoom.42")}</b>
                <span className={g.yourTurn && urgent(g.due, now, g.days) ? "cchallenge__warn" : undefined}>
                  {daysText(g.days)} {t("corrTab.10")} {g.yourTurn ? t("corrTab.11") : t("corrTab.12")}: {timeLeft(g.due, now)} {t("corrTab.13")} {Math.ceil(g.plies / 2)}
                </span>
              </span>
              <a className={`btn${g.yourTurn ? " btn--primary" : ""}`} href={`#online/${g.id}`}>
                {g.yourTurn ? t("corrTab.14") : t("corrTab.15")}
              </a>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

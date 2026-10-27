import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { fetchRatings, seekLimits } from "../online/api.js";
import { useSettings } from "../settings/SettingsContext.jsx";
import { ratingText } from "../online/rating.js";
import useSeek from "../online/useSeek.js";
import Icon from "../ui/Icon.jsx";
import DuelRoom from "./DuelRoom.jsx";
import PuzzleTabs from "./PuzzleTabs.jsx";
import { useT } from "../i18n/index.js";
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
  const t = useT();
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
        <div className="banner banner--error">{t("duelPage.1")}</div>
      </div>
    );
  }
  return (
    <div className="page lobby duel">
      <header className="page__head">
        <h1 className="page__title">{t("menu.duel")}</h1>
        <p className="page__sub">{t("duelPage.3")}</p>
      </header>
      <div className="duel__tabs">
        <PuzzleTabs mode="duel" />
      </div>
      {(error || seek.error) && <div className="banner banner--error">{error || seek.error}</div>}

      <section className="card lobby__card">
        <h2 className="lobby__h">{t("duelPage.4")}</h2>
        <p className="puzzles__big">{mine && mine.games ? ratingText(mine.rating, mine.provisional) : "1200?"}</p>
        <p className="room__meta">
          {mine && mine.games ? t("duelPage.5", { games: mine.games, wins: mine.wins, draws: mine.draws, losses: mine.losses }) : t("duelPage.6")} {t("duelPage.7")}
        </p>

        {searching ? (
          <div className="lobby__search" role="status" aria-live="polite">
            <span className="lobby__spin" aria-hidden="true" />
            <div>
              <p className="lobby__searching">{t("duelPage.8")}</p>
              <p className="lobby__note">
                {seek.status.waited || 0} {t("duelPage.9")}{seek.status.window ? t("duelPage.10", { p: Math.round(seek.status.window) }) : ""}
                {seek.status.count > 1 ? t("duelPage.11", { count: seek.status.count }) : ""}
              </p>
            </div>
            <button type="button" className="btn" onClick={seek.cancel}>
              {t("common.cancel")}
            </button>
          </div>
        ) : (
          <button type="button" className="btn btn--primary lobby__go" onClick={find} disabled={status === "loading"}>
            <Icon name="duel" size={18} /> {t("duelPage.13")}
          </button>
        )}
      </section>

      <section className="card lobby__card">
        <h2 className="lobby__h">{t("vlobby.26")}</h2>
        <ul className="duel__rules">
          <li>{t("duelPage.15")}</li>
          <li>{t("duelPage.16")}</li>
          <li>{t("duelPage.17")}</li>
          <li>{t("duelPage.18")}</li>
        </ul>
      </section>
    </div>
  );
}

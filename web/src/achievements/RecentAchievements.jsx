import { useEffect, useState } from "react";
import { tOr, useT } from "../i18n/index.js";
import AchievementBadge from "./AchievementBadge.jsx";
import { fetchPublic } from "./api.js";
import { latest } from "./logic.js";
import "./achievements.css";

// "Достижения" on a player's page (#user/<id>): the last badges they earned (up to six) and the count.
// Only earned badges are public. On one's own page there is a link to the whole list.
export default function RecentAchievements({ userId, mine = false, limit = 6 }) {
  const t = useT();
  const [state, setState] = useState({ data: null, failed: false });
  useEffect(() => {
    if (!userId) return undefined;
    const ctl = new AbortController();
    setState({ data: null, failed: false });
    fetchPublic(userId, ctl.signal).then(
      (data) => setState({ data, failed: false }),
      (e) => e.name !== "AbortError" && setState({ data: null, failed: true }),
    );
    return () => ctl.abort();
  }, [userId]);

  if (state.failed || !state.data) return null; // a strip that cannot load is better left out than shown broken
  const { data } = state;
  const items = latest(data.items, limit);
  return (
    <section className="card arecent">
      <header>
        <h2>{t("achievements.recent")}</h2>
        <span className="arecent__count">{t("achievements.count", { n: data.unlocked, total: data.total })}</span>
      </header>
      {items.length === 0 ? (
        <p className="arecent__none">{t("achievements.recentNone")}</p>
      ) : (
        <ul className="arecent__list">
          {items.map((it) => {
            const name = tOr(t, `achievements.${it.id}.name`, it.id);
            return (
              <li key={it.id} className="arecent__item" title={tOr(t, `achievements.${it.id}.desc`, "")}>
                <AchievementBadge category={it.category} tier={it.tier} unlocked size={56} title={name} />
                <span className="arecent__name">{name}</span>
              </li>
            );
          })}
        </ul>
      )}
      {mine && (
        <a className="btn btn--ghost arecent__all" href="#profile/achievements">
          {t("achievements.recentAll")}
        </a>
      )}
    </section>
  );
}

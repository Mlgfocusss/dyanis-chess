import { useCallback, useEffect, useState } from "react";
import { formatDate, tOr, useT } from "../i18n/index.js";
import AchievementBadge from "./AchievementBadge.jsx";
import { fetchMine } from "./api.js";
import { groupByCategory, overallPercent, progressPercent, showsBar, tierTotals, TIERS } from "./logic.js";
import "./achievements.css";

// The badges of the signed-in person, loaded when the tab opens: { data, error, loading, reload }.
export function useAchievements(userId) {
  const [state, setState] = useState({ data: null, error: false, loading: true });
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!userId) return undefined;
    const ctl = new AbortController();
    setState((s) => ({ ...s, error: false, loading: true }));
    fetchMine({ signal: ctl.signal }).then(
      (data) => setState({ data, error: false, loading: false }),
      (e) => {
        if (e.name !== "AbortError") setState({ data: null, error: true, loading: false });
      },
    );
    return () => ctl.abort();
  }, [userId, tick]);
  const reload = useCallback(() => setTick((n) => n + 1), []);
  return { ...state, reload };
}

// The texts of a badge by its id; a badge the dictionaries do not know yet shows its id.
export function useBadgeText() {
  const t = useT();
  return (id) => ({ name: tOr(t, `achievements.${id}.name`, id), desc: tOr(t, `achievements.${id}.desc`, "") });
}

function Card({ it }) {
  const t = useT();
  const text = useBadgeText()(it.id);
  const bar = showsBar(it);
  return (
    <li className={`achc achc--${it.tier} ${it.unlocked ? "achc--on" : "achc--off"}`}>
      <span className="achc__badge">
        <AchievementBadge category={it.category} tier={it.tier} unlocked={it.unlocked} size={56} />
      </span>
      <div className="achc__body">
        <h4 className="achc__name">
          {text.name}
          {it.unlocked && !it.seen && <span className="achc__new">{t("achievements.new")}</span>}
        </h4>
        <p className="achc__desc">{text.desc}</p>
        <div className="achc__foot">
          {it.unlocked ? (
            <span className="achc__date">{t("achievements.got", { date: formatDate(it.unlockedAt, { day: "numeric", month: "long", year: "numeric" }) })}</span>
          ) : (
            <span>{bar ? t("achievements.progress", { current: it.progress, target: it.target }) : t("achievements.locked")}</span>
          )}
          {bar && !it.unlocked && (
            <div className="achc__track" role="progressbar" aria-valuemin={0} aria-valuemax={it.target} aria-valuenow={it.progress}>
              <i style={{ width: `${progressPercent(it)}%` }} />
            </div>
          )}
        </div>
      </div>
    </li>
  );
}

// "Профиль → Достижения": the counter, the totals by grade, and the cards by category.
export default function AchievementsTab({ userId }) {
  const t = useT();
  const { data, error, loading, reload } = useAchievements(userId);

  if (error) {
    return (
      <div className="card ach__error" role="alert">
        <span>{t("achievements.loadError")}</span>
        <button type="button" className="btn btn--ghost" onClick={reload}>
          {t("achievements.retry")}
        </button>
      </div>
    );
  }
  if (!data) return <p className="ach__note">{loading ? t("achievements.loading") : ""}</p>;

  const groups = groupByCategory(data.items);
  const tiers = tierTotals(data.items);
  return (
    <div className="ach">
      <section className="card ach__head">
        <div className="ach__headrow">
          <h2>{t("achievements.title")}</h2>
          <span className="ach__count">{t("achievements.count", { n: data.unlocked, total: data.total })}</span>
        </div>
        <div className="ach__bar" role="progressbar" aria-valuemin={0} aria-valuemax={data.total} aria-valuenow={data.unlocked}>
          <i style={{ width: `${overallPercent(data.unlocked, data.total)}%` }} />
        </div>
        <ul className="ach__tiers" style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {TIERS.map((tier) => (
            <li key={tier} className="ach__tier">
              <AchievementBadge category="games" tier={tier} unlocked size={18} />
              {t("achievements.tierLine", { name: t(`achievements.tier.${tier}`), n: tiers[tier].n, total: tiers[tier].total })}
            </li>
          ))}
        </ul>
        <p className="ach__intro">{t("achievements.intro")}</p>
      </section>

      {groups.map((g) => (
        <section key={g.category} className="card ach__group">
          <div className="ach__grouphead">
            <h3>{tOr(t, `achievements.cat.${g.category}`, g.category)}</h3>
            <span className="ach__groupcount">{t("achievements.countShort", { n: g.unlocked, total: g.total })}</span>
          </div>
          <ul className="ach__grid">
            {g.items.map((it) => (
              <Card key={it.id} it={it} />
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

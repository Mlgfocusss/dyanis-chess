import { useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Icon from "../ui/Icon.jsx";
import { tOr, useT } from "../i18n/index.js";
import AchievementBadge from "./AchievementBadge.jsx";
import { fetchMine, markSeen } from "./api.js";
import { MIN_GAP_MS, POLL_MS, TOAST_MS, mayAsk, planToasts } from "./logic.js";
import "./achievements.css";

// The notice "Новое достижение: …" in the top right corner. It asks the server for the badges now and then
// (a minute apart, and after a change of page, but never more than every 15 s; the server also reuses
// its count for a minute), draws a notice for each badge not shown yet and tells the server that it was
// shown. Mounted once, in Root.jsx.
export default function AchievementToasts() {
  const t = useT();
  const { user } = useAuth();
  const userId = user?.id || "";
  const [toasts, setToasts] = useState([]);
  const shown = useRef(new Set()); // ids already drawn in this tab
  const last = useRef(0);
  const timers = useRef(new Map());
  const info = useRef({}); // id -> { category, tier } of the badges of the last answer

  const dismiss = useCallback((key) => {
    clearTimeout(timers.current.get(key));
    timers.current.delete(key);
    setToasts((list) => list.filter((x) => x.key !== key));
  }, []);

  const check = useCallback(async () => {
    const now = Date.now();
    if (!mayAsk(now, last.current)) return;
    last.current = now;
    let data;
    try {
      data = await fetchMine({ poll: true });
    } catch {
      return; // the notice is a nicety: stay quiet when the server is not reachable
    }
    info.current = Object.fromEntries((data.items || []).map((it) => [it.id, it]));
    const plan = planToasts(data.unseen, shown.current);
    if (plan.length === 0) return;
    for (const p of plan) for (const id of p.ids) shown.current.add(id);
    setToasts((list) => [...list, ...plan]);
    for (const p of plan) {
      timers.current.set(p.key, setTimeout(() => dismiss(p.key), TOAST_MS));
    }
    markSeen(plan.flatMap((p) => p.ids)).catch(() => {});
  }, [dismiss]);

  useEffect(() => {
    if (!userId) {
      setToasts([]);
      shown.current = new Set();
      last.current = 0;
      return undefined;
    }
    check();
    const poll = setInterval(() => {
      if (document.visibilityState === "visible") check();
    }, POLL_MS);
    // a game or a puzzle ends and the person moves on: ask soon after the page changes
    let later = 0;
    const onHash = () => {
      clearTimeout(later);
      later = setTimeout(check, Math.max(1500, MIN_GAP_MS - (Date.now() - last.current)));
    };
    const onVisible = () => document.visibilityState === "visible" && check();
    window.addEventListener("hashchange", onHash);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(poll);
      clearTimeout(later);
      window.removeEventListener("hashchange", onHash);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [userId, check]);

  useEffect(() => {
    const m = timers.current;
    return () => {
      for (const id of m.values()) clearTimeout(id);
      m.clear();
    };
  }, []);

  if (toasts.length === 0) return null;
  return (
    <div className="atoasts" role="region" aria-label={t("achievements.toast.region")} aria-live="polite">
      {toasts.map((x) => {
        const it = info.current[x.id];
        return (
          <div key={x.key} className="atoast">
            {x.many ? (
              <Icon name="trophy" size={28} />
            ) : (
              <AchievementBadge category={it?.category} tier={it?.tier || "bronze"} unlocked size={36} />
            )}
            <a className="atoast__text" href="#profile/achievements" onClick={() => dismiss(x.key)}>
              <b>{t("achievements.title")}</b>
              <span>
                {x.many
                  ? t("achievements.toast.many", { n: x.many })
                  : t("achievements.toast.one", { name: tOr(t, `achievements.${x.id}.name`, x.id) })}
              </span>
            </a>
            <button type="button" className="atoast__x" aria-label={t("achievements.toast.close")} onClick={() => dismiss(x.key)}>
              <Icon name="close" size={16} />
            </button>
          </div>
        );
      })}
    </div>
  );
}

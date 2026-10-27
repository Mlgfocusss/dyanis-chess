import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { fetchChallenges, fetchSummary } from "./api.js";
import { toNotify } from "./logic.js";
import { useCorrNotify } from "../notify/useCorrNotify.js";
import { tr } from "../i18n/index.js";

// What waits for the person (friend requests, messages, challenges, correspondence games to move in) and
// the small notices that announce it. The menu badge reads `summary`; the pages call `refresh` after they
// change something so that the numbers do not lag. The summary is asked for every POLL_MS while the page
// is in front: that request is also how the others see the person is online.
const POLL_MS = 15000;

const Ctx = createContext({ summary: null, refresh: () => {}, notices: [], notify: () => {}, dismiss: () => {} });

export const useCommunity = () => useContext(Ctx);

// Ids already announced in this browser tab, so a reload does not announce the same challenge again.
const SEEN_KEY = "dyanis.community.seen";
function loadSeen() {
  try {
    return new Set(JSON.parse(window.sessionStorage.getItem(SEEN_KEY)) || []);
  } catch {
    return new Set();
  }
}
function saveSeen(set) {
  try {
    window.sessionStorage.setItem(SEEN_KEY, JSON.stringify([...set].slice(-100)));
  } catch {
    // private mode: the notices may repeat after a reload, nothing worse
  }
}

let noticeId = 0;

export function CommunityProvider({ children }) {
  const { user } = useAuth();
  const { settings } = useSettings();
  const [summary, setSummary] = useState(null);
  const [notices, setNotices] = useState([]);
  const seen = useRef(loadSeen());
  const modeRef = useRef(settings.challengeNotify);
  modeRef.current = settings.challengeNotify;
  const userId = user?.id || null;

  const dismiss = useCallback((id) => setNotices((l) => l.filter((n) => n.id !== id)), []);
  const notify = useCallback(
    (n) => {
      const id = ++noticeId;
      setNotices((l) => [...l.slice(-3), { id, ...n }]);
      setTimeout(() => dismiss(id), n.ms || 14000);
    },
    [dismiss],
  );

  const refresh = useCallback(async () => {
    if (!userId) return;
    try {
      const s = await fetchSummary();
      setSummary(s);
      // A challenge I sent was accepted: the game is ready.
      for (const a of s.accepted || []) {
        const key = "acc:" + a.id;
        if (seen.current.has(key)) continue;
        seen.current.add(key);
        notify({ text: tr("commContext.1"), href: `#online/${a.gameId}`, action: tr("botsPage.play") });
      }
      // New challenges to me, announced as the settings say.
      if (s.challenges > 0 && modeRef.current !== "none") {
        const { incoming } = await fetchChallenges();
        const fresh = toNotify(incoming, seen.current, modeRef.current);
        for (const c of fresh) {
          seen.current.add(c.id);
          notify({ text: tr("commContext.3", { name: c.person.name }), href: "#community/challenges", action: tr("commContext.4") });
        }
        // Those not announced (a friend-only mode) are still "seen enough" to not be asked about again.
        incoming.forEach((c) => seen.current.add(c.id));
      }
      saveSeen(seen.current);
    } catch {
      // offline or signed out: the badge keeps what it had
    }
  }, [userId, notify]);

  useEffect(() => {
    setSummary(null);
    if (!userId) return undefined;
    refresh();
    const tick = () => {
      if (!document.hidden) refresh();
    };
    const t = setInterval(tick, POLL_MS);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", tick);
    };
  }, [userId, refresh]);

  useCorrNotify(summary, userId, notify); // title, sound and notice for a move in a correspondence game (notify/)

  const value = useMemo(() => ({ summary, refresh, notices, notify, dismiss }), [summary, refresh, notices, notify, dismiss]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

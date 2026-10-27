import { useEffect, useRef } from "react";
import { useSettings } from "../settings/SettingsContext.jsx";
import { useT } from "../i18n/index.js";
import { fetchCorrespondence, fetchSummary } from "../community/api.js";
import { playSound, unlockAudio } from "../sound/sound.js";
import { countWentUp, freshTurns, openRoomId, titleWithCount } from "./logic.js";
import { showSystemNotice } from "./system.js";

// While the tab is in the background the community summary is not asked for (CommunityContext), so a person
// with correspondence games asks for it here, less often, to hear of a move at all.
const BG_POLL_MS = 20000;

const fresh = () => ({ count: null, known: new Map() });

// The notice about a move in a correspondence game. Called from CommunityProvider with its summary
// (`{ yourTurn, ... }`, null while there is none), the id of the signed-in person and the provider's
// `notify` (the small notice in the corner).
//   - the tab title carries the number of games waiting for a move: "(2) Dyanis Chess";
//   - when that number goes up: a sound (settings corrNotifySound and the general sound), a system notice
//     (corrNotifyBrowser, once the browser has allowed them, and only while the tab is hidden) or, with the
//     tab in view, the small notice in the corner. Nothing when the game with that move is the one open on
//     the screen: the room announces its own moves.
export function useCorrNotify(summary, userId, toast) {
  const { settings } = useSettings();
  const t = useT();
  const live = useRef({});
  live.current = { settings, t, toast };
  const st = useRef(fresh());

  // The work itself, kept in a ref so that the effects below always call the latest one.
  const handle = useRef(null);
  handle.current = (count) => {
    const s = st.current;
    const prev = s.count;
    if (prev === count) return;
    s.count = count;
    document.title = titleWithCount(document.title, count);
    if (prev === null) {
      // the first answer: what is waiting is remembered, not announced
      if (count > 0) fetchCorrespondence().then((list) => (s.known = freshTurns(list, new Map()).known), () => {});
      return;
    }
    if (countWentUp(prev, count)) announce(s, live.current);
  };

  // a new person (or none): forget everything and take the number off the title
  useEffect(() => {
    st.current = fresh();
    return () => {
      st.current = fresh();
      document.title = titleWithCount(document.title, 0);
    };
  }, [userId]);

  useEffect(() => {
    if (summary) handle.current(summary.yourTurn || 0);
  }, [summary]);

  // a browser may let a sound play only after the person has done something on the page
  useEffect(() => {
    if (!userId) return undefined;
    const unlock = () => unlockAudio();
    window.addEventListener("pointerdown", unlock, { once: true });
    window.addEventListener("keydown", unlock, { once: true });
    return () => {
      window.removeEventListener("pointerdown", unlock);
      window.removeEventListener("keydown", unlock);
    };
  }, [userId]);

  // the tab goes to the background: keep asking, but only if there is a correspondence game to wait for
  useEffect(() => {
    if (!userId) return undefined;
    let timer = null;
    let gone = false;
    const stop = () => {
      clearInterval(timer);
      timer = null;
    };
    const start = async () => {
      let games = [];
      try {
        games = await fetchCorrespondence();
      } catch {
        return;
      }
      if (gone || !document.hidden || games.length === 0 || timer) return;
      // bring the count up to date first: a move made a moment ago has already lowered it
      try {
        handle.current((await fetchSummary()).yourTurn || 0);
      } catch {
        // the poll below tries again
      }
      if (gone || !document.hidden || timer) return;
      timer = setInterval(async () => {
        if (!document.hidden) return stop();
        try {
          handle.current((await fetchSummary()).yourTurn || 0);
        } catch {
          // offline: try again at the next tick
        }
      }, BG_POLL_MS);
    };
    const onVisibility = () => (document.hidden ? start() : stop());
    document.addEventListener("visibilitychange", onVisibility);
    if (document.hidden) start();
    return () => {
      gone = true;
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [userId]);
}

async function announce(s, { settings, t, toast }) {
  let list = null; // the games that are a new call for a move; null: the list could not be had
  try {
    const r = freshTurns(await fetchCorrespondence(), s.known);
    s.known = r.known;
    list = r.fresh;
  } catch {
    // the notice is made without the name
  }
  const wantSound = settings.sound && settings.corrNotifySound;
  const wantSystem = settings.corrNotifyBrowser;
  if (!wantSound && !wantSystem) return;
  const visible = !document.hidden;
  if (list) {
    const room = visible ? openRoomId(window.location.hash) : "";
    list = list.filter((g) => g.id !== room);
    if (list.length === 0) return;
  }
  const one = list && list.length === 1 ? list[0] : null;
  const text = !list
    ? t("notify.bodyAnon")
    : list.length > 1
      ? t("notify.bodyMany", { n: list.length })
      : one.opponent?.name
        ? t("notify.body", { name: one.opponent.name })
        : t("notify.bodyAnon");
  const href = one ? `#online/${one.id}` : "#community/correspondence";
  if (wantSound) playSound("notify", settings.soundTheme);
  if (!visible) {
    if (wantSystem) showSystemNotice(text, href);
  } else if (toast) {
    toast({ text, href, action: t("notify.action") });
  }
}

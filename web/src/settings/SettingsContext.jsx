import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { saveSettings } from "../auth/api.js";
import { backgroundCss, getBackground } from "../themes/index.js";
import { playSound } from "../sound/sound.js";
import { soundKindForSan } from "../sound/kinds.js";

// Everything the user can change on the settings page (see model.js for the
// list). Saved in this browser (localStorage) and, for a signed-in user, on
// the server as well (User.settings), so the choice follows them to another
// device: at sign-in the server's copy wins; a user who has none yet gets
// this browser's settings uploaded.
import { ANIMATION_MS, DEFAULTS, sanitize } from "./model.js";

export { ANIMATION_MS, DEFAULTS, sanitize };

const KEY = "dyanis.settings.v1";

function load() {
  try {
    return sanitize(JSON.parse(window.localStorage.getItem(KEY)));
  } catch {
    return { ...DEFAULTS };
  }
}

const Ctx = createContext(null);

// Writes to the server wait this long after the last change (a slider-like
// sequence of clicks becomes one request).
const UPLOAD_DELAY_MS = 600;

export function SettingsProvider({ children }) {
  const [settings, setSettings] = useState(load);
  const { user } = useAuth();
  const userId = user?.id ?? null;
  const userIdRef = useRef(userId);
  userIdRef.current = userId;
  const upload = useRef(null);
  const pending = useRef(null); // settings waiting for their delayed upload

  const store = useCallback((clean) => {
    try {
      window.localStorage.setItem(KEY, JSON.stringify(clean));
    } catch {
      // private mode or full storage: the choice still applies until reload
    }
  }, []);

  const save = useCallback(
    (next) => {
      const clean = sanitize(next);
      setSettings(clean);
      store(clean);
      if (userIdRef.current) {
        clearTimeout(upload.current);
        pending.current = clean;
        upload.current = setTimeout(() => {
          pending.current = null;
          saveSettings(clean).catch(() => {});
        }, UPLOAD_DELAY_MS);
      }
    },
    [store]
  );

  // At sign-in: the server's settings replace the local ones; if the server
  // has none yet, send ours. Keyed on the user id so it runs once per user.
  const syncedFor = useRef(null);
  useEffect(() => {
    if (!user) {
      syncedFor.current = null;
      return;
    }
    if (syncedFor.current === user.id) return;
    syncedFor.current = user.id;
    if (user.settings && Object.keys(user.settings).length > 0) {
      const clean = sanitize(user.settings);
      setSettings(clean);
      store(clean);
    } else {
      saveSettings(sanitize(settings)).catch(() => {});
    }
    // settings is read only for the first upload, not a trigger
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user, store]);

  // Leaving the page within the delay must not lose the last change.
  useEffect(() => {
    const flush = () => {
      if (pending.current) saveSettings(pending.current).catch(() => {});
      pending.current = null;
    };
    window.addEventListener("pagehide", flush);
    return () => window.removeEventListener("pagehide", flush);
  }, []);

  // Another tab changed the settings.
  useEffect(() => {
    function onStorage(e) {
      if (e.key === KEY) setSettings(load());
    }
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  // The page background belongs to the document, not to one component.
  useEffect(() => {
    const css = backgroundCss(getBackground(settings.background));
    const s = document.body.style;
    s.backgroundImage = css === "none" ? "" : css;
    s.backgroundSize = "cover";
    s.backgroundAttachment = "fixed";
    s.backgroundPosition = "center";
  }, [settings.background]);

  const value = useMemo(() => ({ settings, save }), [settings, save]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSettings() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useSettings outside SettingsProvider");
  return v;
}

// Plays the sound of a move (by its SAN) if sound is on.
export function useMoveSound() {
  const { settings } = useSettings();
  return useCallback(
    (san) => {
      if (settings.sound && san) playSound(soundKindForSan(san), settings.soundTheme);
    },
    [settings.sound, settings.soundTheme]
  );
}

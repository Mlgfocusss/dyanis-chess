import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { backgroundCss, getBackground } from "../themes/index.js";
import { playSound } from "../sound/sound.js";
import { soundKindForSan } from "../sound/kinds.js";

// Everything the user can change on the settings page (see model.js for the
// list). Saved in this browser (localStorage); when accounts arrive
// (WEB-PLAN.md, stage 0) the same object becomes User.settings on the server.
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

export function SettingsProvider({ children }) {
  const [settings, setSettings] = useState(load);

  const save = useCallback((next) => {
    const clean = sanitize(next);
    setSettings(clean);
    try {
      window.localStorage.setItem(KEY, JSON.stringify(clean));
    } catch {
      // private mode or full storage: the choice still applies until reload
    }
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

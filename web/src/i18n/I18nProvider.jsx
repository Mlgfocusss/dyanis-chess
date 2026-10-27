import { useLayoutEffect, useMemo } from "react";
import { I18nContext, setLanguage, translate, isLanguage } from "./index.js";
import { useSettings } from "../settings/SettingsContext.jsx";

// Puts the language of the saved settings under the whole site: `useT()` gives the translate
// function, <html lang> follows. The language of the pure helpers (`tr`) is set while drawing, before
// the children are drawn, so no child sees a stale one.
export function I18nProvider({ lang, children }) {
  const l = isLanguage(lang) ? lang : "ru";
  setLanguage(l);
  useLayoutEffect(() => {
    document.documentElement.setAttribute("lang", l);
  }, [l]);
  const value = useMemo(() => ({ lang: l, t: (key, params) => translate(l, key, params) }), [l]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

// The same, reading the language from the settings (must stand inside SettingsProvider).
export function SettingsI18n({ children }) {
  const { settings } = useSettings();
  return <I18nProvider lang={settings.language}>{children}</I18nProvider>;
}

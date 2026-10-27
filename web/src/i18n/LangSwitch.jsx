import { useSettings } from "../settings/SettingsContext.jsx";
import { LANGUAGES, useT } from "./index.js";
import "./i18n.css";

const NAMES = { ru: "Русский", en: "English" };

// A small "RU | EN" switch for the menu: the language changes at once and is saved with the settings
// (and on the server for a signed-in person). The same choice is on "Настройки → Интерфейс".
export default function LangSwitch({ className = "" }) {
  const t = useT();
  const { settings, save } = useSettings();
  return (
    <div className={`langsw${className ? " " + className : ""}`} role="radiogroup" aria-label={t("lang.label")}>
      {LANGUAGES.map((l) => (
        <button
          key={l}
          type="button"
          role="radio"
          lang={l}
          aria-checked={settings.language === l}
          title={NAMES[l]}
          className={`langsw__opt${settings.language === l ? " langsw__opt--on" : ""}`}
          onClick={() => settings.language !== l && save({ ...settings, language: l })}
        >
          {l.toUpperCase()}
        </button>
      ))}
    </div>
  );
}

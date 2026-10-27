import Segmented from "../ui/Segmented.jsx";
import Icon from "../ui/Icon.jsx";
import { Row, SaveBar, Switch } from "./controls.jsx";
import { LANGUAGES, useLang, useT } from "../i18n/index.js";
import { useSettings } from "./SettingsContext.jsx";
import "../i18n/i18n.css";
import "./theme.css";

const THEMES = [
  ["dark", "interface.themeDark"],
  ["light", "interface.themeLight"],
  ["system", "interface.themeSystem"],
];

// A small picture of the page in one theme (colours: the --pv-* constants, see theme.css).
function Preview({ theme }) {
  const face = (kind, cut) => (
    <div className={`themepv__face themepv__face--${kind}${cut ? " themepv__face--cut" : ""}`}>
      <div className="themepv__menu" />
      <div className="themepv__card">
        <div className="themepv__line" />
        <div className="themepv__line themepv__line--short" />
        <div className="themepv__btn" />
      </div>
    </div>
  );
  return (
    <div className="themepv" aria-hidden="true">
      {theme === "dark" ? face("dark") : face("light")}
      {theme === "system" && face("dark", true)}
    </div>
  );
}

function ThemePicker({ value, onChange }) {
  const t = useT();
  return (
    <div className="themepick" role="radiogroup" aria-label={t("interface.theme")}>
      {THEMES.map(([id, name]) => (
        <button
          key={id}
          type="button"
          role="radio"
          aria-checked={value === id}
          className={`themeopt${value === id ? " themeopt--on" : ""}`}
          onClick={() => onChange(id)}
        >
          <Preview theme={id} />
          <span className="themeopt__name">
            {t(name)}
            {value === id && <Icon name="check" />}
          </span>
        </button>
      ))}
    </div>
  );
}

// "Интерфейс": how the whole site looks and moves. The choices are previewed at once (SettingsPage
// puts the draft on <html>) and kept only after "Сохранить".
// The choice of the language: applied and saved at once (it is not a draft like the rest of the page).
function LanguagePicker({ value, onChange }) {
  const t = useT();
  return (
    <div className="langpick" role="radiogroup" aria-label={t("lang.label")}>
      {LANGUAGES.map((l) => (
        <button
          key={l}
          type="button"
          role="radio"
          lang={l}
          aria-checked={value === l}
          className={`langopt${value === l ? " langopt--on" : ""}`}
          onClick={() => onChange(l)}
        >
          <span className="langopt__code">{l.toUpperCase()}</span>
          <span className="langopt__name">
            {l === "ru" ? "Русский" : "English"}
            {value === l && <Icon name="check" />}
          </span>
        </button>
      ))}
    </div>
  );
}

export default function InterfacePanel({ draft, set, dirty, justSaved, onSave, onCancel }) {
  const t = useT();
  const { settings, save } = useSettings();
  return (
    <>
      <h2 className="settings__h">{t("interface.6")}</h2>
      <p className="settings__sub">{t("interface.7")}</p>

      <Row title={t("lang.title")} hint={t("lang.hint")}>
        <LanguagePicker
          value={draft.language}
          onChange={(l) => {
            set({ language: l });
            save({ ...settings, language: l });
          }}
        />
      </Row>

      <Row
        title={t("interface.theme")}
        hint={t("interface.textSize")}
      >
        <ThemePicker value={draft.theme} onChange={(v) => set({ theme: v })} />
      </Row>

      <Row title={t("interface.9")} hint={t("interface.10")}>
        <Segmented
          label={t("interface.9")}
          value={draft.textSize}
          onChange={(v) => set({ textSize: v })}
          options={[
            ["normal", t("interface.11")],
            ["large", t("interface.12")],
          ]}
        />
      </Row>

      <Row
        title={t("interface.13")}
        hint={t("interface.14")}
        control={<Switch checked={draft.reduceMotion} onChange={(v) => set({ reduceMotion: v })} label={t("interface.13")} />}
      />

      <Row
        title={t("interface.15")}
        hint={t("interface.16")}
        control={<Switch checked={draft.highContrast} onChange={(v) => set({ highContrast: v })} label={t("interface.15")} />}
      >
        <div className="contrast-demo" aria-hidden="true">
          <span className="contrast-demo__box">{t("interface.17")}</span>
          <span className="contrast-demo__box contrast-demo__box--field">{t("interface.18")}</span>
        </div>
      </Row>

      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
    </>
  );
}

import Segmented from "../ui/Segmented.jsx";
import Icon from "../ui/Icon.jsx";
import { Row, SaveBar, Switch } from "./controls.jsx";
import "./theme.css";

const THEMES = [
  ["dark", "Тёмная"],
  ["light", "Светлая"],
  ["system", "Как в системе"],
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
  return (
    <div className="themepick" role="radiogroup" aria-label="Тема">
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
            {name}
            {value === id && <Icon name="check" />}
          </span>
        </button>
      ))}
    </div>
  );
}

// "Интерфейс": how the whole site looks and moves. The choices are previewed at once (SettingsPage
// puts the draft on <html>) and kept only after "Сохранить".
export default function InterfacePanel({ draft, set, dirty, justSaved, onSave, onCancel }) {
  return (
    <>
      <h2 className="settings__h">Интерфейс</h2>
      <p className="settings__sub">Тема, размер текста, анимации и контраст на всём сайте. Изменения видны сразу, а сохраняются по кнопке.</p>

      <Row
        title="Тема"
        hint="Светлая или тёмная страница. «Как в системе» повторяет настройку вашего устройства и меняется вместе с ней. Доски и фигуры от темы не зависят; фон страницы из раздела «Доска и фигуры» виден только в тёмной теме."
      >
        <ThemePicker value={draft.theme} onChange={(v) => set({ theme: v })} />
      </Row>

      <Row title="Размер текста" hint="Крупный текст читается легче: надписи, меню и подсказки на всём сайте становятся примерно на 13% больше.">
        <Segmented
          label="Размер текста"
          value={draft.textSize}
          onChange={(v) => set({ textSize: v })}
          options={[
            ["normal", "Обычный"],
            ["large", "Крупный"],
          ]}
        />
      </Row>

      <Row
        title="Уменьшить анимации"
        hint="Без плавных переходов и движения интерфейса, в том числе скольжения фигур по доске. Включается и само, если так настроена ваша система."
        control={<Switch checked={draft.reduceMotion} onChange={(v) => set({ reduceMotion: v })} label="Уменьшить анимации" />}
      />

      <Row
        title="Контрастные границы"
        hint="Границы карточек, полей и разделителей становятся ярче, чтобы части страницы лучше отделялись друг от друга."
        control={<Switch checked={draft.highContrast} onChange={(v) => set({ highContrast: v })} label="Контрастные границы" />}
      >
        <div className="contrast-demo" aria-hidden="true">
          <span className="contrast-demo__box">Карточка</span>
          <span className="contrast-demo__box contrast-demo__box--field">Поле ввода</span>
        </div>
      </Row>

      <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
    </>
  );
}

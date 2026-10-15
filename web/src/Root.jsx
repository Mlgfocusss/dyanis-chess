import { useState } from "react";
import App from "./App.jsx";
import ReviewPage from "./review/ReviewPage.jsx";
import SettingsPage from "./settings/SettingsPage.jsx";
import { SettingsProvider } from "./settings/SettingsContext.jsx";
import Icon from "./ui/Icon.jsx";
import "./Root.css";

// Pages that exist, then the ones on the roadmap (WEB-PLAN.md), shown
// disabled so the product's shape is visible.
const ITEMS = [
  { id: "review", label: "Разбор партии", icon: "review" },
  { id: "play", label: "Игра с ботом", icon: "bot" },
  { id: "online", label: "Онлайн", icon: "online", soon: true },
  { id: "puzzles", label: "Задачи", icon: "puzzles", soon: true },
  { id: "openings", label: "Дебюты", icon: "openings", soon: true },
  { id: "coach", label: "Тренер", icon: "coach", soon: true },
];
const BOTTOM = [{ id: "settings", label: "Настройки", icon: "sliders" }];
const PAGES = [...ITEMS, ...BOTTOM].filter((i) => !i.soon).map((i) => i.id);

// The page is kept in the URL hash (#play, #review/<id>) so a reload or a
// shared link lands on the same page; a router can replace this once
// there are pages with parameters.
function initialPage() {
  const h = window.location.hash.slice(1).split("/")[0];
  return PAGES.includes(h) ? h : "review";
}

function NavList({ items, page, select, className = "" }) {
  return (
    <ul className={`nav__list ${className}`}>
      {items.map((it) => (
        <li key={it.id} className={it.soon ? "nav__soon-item" : undefined}>
          <button
            type="button"
            className={`nav__item${page === it.id ? " nav__item--active" : ""}`}
            onClick={() => select(it.id)}
            disabled={it.soon}
            aria-current={page === it.id ? "page" : undefined}
          >
            <Icon name={it.icon} />
            <span className="nav__label">{it.label}</span>
            {it.soon && <span className="nav__soon">скоро</span>}
          </button>
        </li>
      ))}
    </ul>
  );
}

export default function Root() {
  const [page, setPage] = useState(initialPage);

  function select(id) {
    setPage(id);
    window.location.hash = id;
  }

  return (
    <SettingsProvider>
      <div className="shell">
        <nav className="nav" aria-label="Основное меню">
          <div className="nav__brand">
            <span className="nav__logo">
              <Icon name="crown" size={22} />
            </span>
            <span className="nav__name">Dyanis Chess</span>
          </div>

          <NavList items={ITEMS} page={page} select={select} />
          <NavList items={BOTTOM} page={page} select={select} className="nav__list--bottom" />

          <p className="nav__foot">
            Свой движок на Go
            <br />и нейросеть NNUE
          </p>
        </nav>

        <main className="main">
          {/* Only the visible page is mounted: the play page owns a Web Worker. */}
          {page === "review" && <ReviewPage />}
          {page === "play" && <App />}
          {page === "settings" && <SettingsPage />}
        </main>
      </div>
    </SettingsProvider>
  );
}

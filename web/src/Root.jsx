import { useEffect, useRef, useState } from "react";
import App from "./App.jsx";
import HomePage from "./home/HomePage.jsx";
import ReviewPage from "./review/ReviewPage.jsx";
import AnalysisPage from "./review/AnalysisPage.jsx";
import SettingsPage from "./settings/SettingsPage.jsx";
import AccountPage from "./auth/AccountPage.jsx";
import ProfilePage from "./profile/ProfilePage.jsx";
import OnlinePage from "./online/OnlinePage.jsx";
import BotsPage from "./bots/BotsPage.jsx";
import PuzzlesPage from "./puzzles/PuzzlesPage.jsx";
import RushPage from "./puzzles/RushPage.jsx";
import DuelPage from "./puzzles/DuelPage.jsx";
import OpeningsPage from "./openings/OpeningsPage.jsx";
import LessonsPage from "./lessons/LessonsPage.jsx";
import TrainerPage from "./trainer/TrainerPage.jsx";
import LeadersPage from "./leaders/LeadersPage.jsx";
import UserPage from "./leaders/UserPage.jsx";
import VariantsPage from "./variants/VariantsPage.jsx";
import { VARIANTS } from "./variants/registry.js";
import AccountCard from "./auth/AccountCard.jsx";
import { AuthProvider, useAuth } from "./auth/AuthContext.jsx";
import { SettingsProvider, useSettings } from "./settings/SettingsContext.jsx";
import Icon from "./ui/Icon.jsx";
import "./Root.css";

// The menu, in order of how often it is wanted: play first (with the three
// kinds of opponent), then puzzles (rated ones, by theme, the puzzle of the
// day), openings and lessons, then the review.
// Items marked soon are on the roadmap (WEB-PLAN.md) and shown disabled so the
// shape of the product is visible. The profile sits at the bottom left, in the
// account card (see auth/AccountCard.jsx).
const NAV = [
  { id: "home", label: "Главная", icon: "home" },
  {
    id: "play-menu",
    label: "Играть",
    icon: "play",
    children: [
      { id: "online", label: "Играть онлайн", icon: "online" },
      { id: "bots", label: "Играть с ботами", icon: "bots" },
      { id: "play", label: "Играть против движка", icon: "bot" },
      { id: "leaders", label: "Таблица лидеров", icon: "trophy" },
    ],
  },
  {
    // "Другие шахматы": the pages of the variants come from their folders (web/src/variants/<id>/)
    id: "variants-menu",
    label: "Другие шахматы",
    icon: "variants",
    children: [
      { id: "variants", label: "Все виды", icon: "list" },
      ...VARIANTS.map((v) => ({ id: `variant:${v.id}`, label: v.name, icon: v.icon })),
    ],
  },
  {
    id: "puzzles-menu",
    label: "Задачи",
    icon: "puzzles",
    children: [
      { id: "puzzles", label: "Рейтинговые", icon: "puzzles" },
      { id: "puzzles-themes", label: "По темам", icon: "list" },
      { id: "puzzles-daily", label: "Задача дня", icon: "trophy" },
      { id: "puzzles-rush", label: "Тактический штурм", icon: "bolt" },
      { id: "puzzles-duel", label: "Тактическая дуэль", icon: "duel" },
    ],
  },
  { id: "openings", label: "Дебюты", icon: "openings" },
  { id: "lessons", label: "Уроки", icon: "lessons" },
  { id: "review", label: "Разбор партии", icon: "review" },
  { id: "analysis", label: "Доска анализа", icon: "board", noMobile: true }, // on the bottom bar there is no room: the review page links to it
  { id: "coach", label: "Тренер", icon: "coach" },
];
const SETTINGS_ITEM = { id: "settings", label: "Настройки", icon: "sliders" };
const PAGES = ["home", "review", "analysis", "lessons", "coach", "online", "bots", "play", "puzzles", "variants", "openings", "leaders", "user", "profile", "account", "settings"];
// The puzzle pages share one page and differ in the hash: [page, hash] of a menu item. So do the variants.
const SUB_PAGES = {
  ...Object.fromEntries(VARIANTS.map((v) => [`variant:${v.id}`, ["variants", `variants/${v.id}`]])),
  "puzzles-themes": ["puzzles", "puzzles/themes"],
  "puzzles-daily": ["puzzles", "puzzles/daily"],
  "puzzles-rush": ["puzzles", "puzzles/rush"],
  "puzzles-duel": ["puzzles", "puzzles/duel"],
};
// Old addresses that moved.
const ALIASES = { games: "profile" };

// The page is kept in the URL hash (#play, #review/<id>) so a reload or a
// shared link lands on the same page; a router can replace this once
// there are pages with parameters.
function initialPage() {
  const raw = window.location.hash.slice(1).split("/")[0];
  const h = ALIASES[raw] || raw;
  return PAGES.includes(h) ? h : "home";
}

// The menu item a page lights up: the puzzle tabs have their own.
function menuId(page) {
  if (page === "user") return "leaders"; // a player's page belongs to the leaderboard
  if (page === "variants") {
    const part = window.location.hash.slice(1).split("/")[1];
    return VARIANTS.some((v) => v.id === part) ? `variant:${part}` : "variants";
  }
  if (page !== "puzzles") return page;
  const part = window.location.hash.slice(1).split("/")[1];
  return part === "themes" || part === "daily" || part === "rush" || part === "duel" ? `puzzles-${part}` : "puzzles";
}

// "rush" and "duel" are pages of their own inside the puzzle section (the address #puzzles/rush).
function puzzleSection() {
  const part = window.location.hash.slice(1).split("/")[1];
  return part === "rush" || part === "duel" ? part : "";
}

function Item({ it, active, onClick, open, ...rest }) {
  return (
    <button
      type="button"
      className={"nav__item" + (active ? " nav__item--active" : "")}
      onClick={onClick}
      disabled={it.soon}
      aria-current={active && !it.children ? "page" : undefined}
      aria-expanded={it.children ? !!open : undefined}
      aria-haspopup={it.children ? "menu" : undefined}
      {...rest}
    >
      <Icon name={it.icon} />
      <span className="nav__label">{it.label}</span>
      {it.soon && <span className="nav__soon">скоро</span>}
      {it.children && <Icon name="chevron" size={16} className="nav__chev" />}
    </button>
  );
}

// A group of the menu is one button; its pages come out in a popup beside it while the pointer is
// on the button or the popup (on a touch screen: after a tap). flyout is { id, top, by } or null.
function NavList({ items, page, select, className = "", flyout, setFlyout }) {
  const timer = useRef(null);
  const show = (id, el, by) => {
    clearTimeout(timer.current);
    setFlyout((f) => (f && f.id === id && by === "hover" ? f : { id, top: Math.round(el.getBoundingClientRect().top), by }));
  };
  const hideSoon = () => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setFlyout(null), 160);
  };
  useEffect(() => () => clearTimeout(timer.current), []);
  return (
    <ul className={"nav__list " + className}>
      {items.map((it) => {
        const li = it.soon || it.noMobile ? "nav__soon-item" : it.mobileOnly ? "nav__mobile-only" : undefined;
        if (!it.children) {
          return (
            <li key={it.id} className={li}>
              <Item it={it} active={page === it.id} onClick={() => select(it.id)} />
            </li>
          );
        }
        // It looks active while one of its pages is shown.
        const inside = it.children.some((c) => c.id === page);
        const open = flyout?.id === it.id;
        return (
          <li key={it.id} className="nav__group" onMouseLeave={hideSoon} onMouseEnter={(e) => show(it.id, e.currentTarget, "hover")} data-flyout-group>
            <Item
              it={it}
              active={inside}
              open={open}
              onClick={(e) => (open && flyout.by === "click" ? setFlyout(null) : show(it.id, e.currentTarget.parentElement, "click"))}
            />
            {open && (
              <ul className="nav__sub" role="menu" style={{ "--fly-top": flyout.top + "px" }}>
                {it.children.map((c) => (
                  <li key={c.id} className={c.soon ? "nav__soon-item" : undefined} role="none">
                    <Item it={c} active={page === c.id} onClick={() => select(c.id)} role="menuitem" />
                  </li>
                ))}
              </ul>
            )}
          </li>
        );
      })}
    </ul>
  );
}

// Pages with a board that the focus mode makes sense for: the game pages only
// while a game is on (#online/<id>, #bots/<id>), not in their lobbies.
function pageHasBoard(page) {
  if (page === "review" || page === "analysis" || page === "play" || page === "puzzles") return true;
  if (page === "openings") return !!window.location.hash.slice(1).split("/")[1];
  if (page === "online" || page === "bots") return !!window.location.hash.slice(1).split("/")[1];
  if (page === "variants") return window.location.hash.slice(1).split("/")[1] === "game"; // a game, not the lobby
  return false;
}

function Shell() {
  const { user, authError } = useAuth();
  const { settings, save } = useSettings();
  // A failed Google sign-in lands on the account page, where the reason is shown.
  const [page, setPage] = useState(() => (authError ? "account" : initialPage()));

  const [flyout, setFlyout] = useState(null); // the group whose pages are shown in a popup
  const [, redraw] = useState(0); // the hash alone can change what the page offers (a game opens)
  const focus = settings.focusBoard;
  const setFocus = (v) => save({ ...settings, focusBoard: v });
  function select(id) {
    setFlyout(null);
    const [pg, hash] = SUB_PAGES[id] || [id, id];
    setPage(pg);
    window.location.hash = hash;
  }

  // Links such as <a href="#review/<id>"> change the hash without our help.
  useEffect(() => {
    const onHash = () => {
      setPage(initialPage());
      setFlyout(null);
      redraw((n) => n + 1);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // A press outside the menu closes the popup (on a touch screen there is no pointer leaving it).
  useEffect(() => {
    if (!flyout) return undefined;
    const onDown = (e) => {
      if (!e.target.closest("[data-flyout-group]")) setFlyout(null);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [flyout]);

  const canFocus = pageHasBoard(page);
  const focused = focus && canFocus;
  // Escape leaves the focus mode.
  useEffect(() => {
    if (!focused) return undefined;
    const onKey = (e) => e.key === "Escape" && setFocus(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // setFocus only reads the settings at the moment of the key
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focused, settings]);

  const accountItem = user
    ? { id: "profile", label: "Профиль", icon: "user", mobileOnly: true }
    : { id: "account", label: "Войти", icon: "user", mobileOnly: true };

  return (
    <div className={`shell${focused ? " shell--focus" : ""}`}>
      {canFocus && (
        <button
          type="button"
          className="focus-toggle"
          onClick={() => setFocus(!focus)}
          title={focus ? "Вернуть меню и панели (Esc)" : "Доска в центре: оставить только доску"}
          aria-label={focus ? "Выйти из режима «Доска в центре»" : "Включить режим «Доска в центре»"}
          aria-pressed={focus}
        >
          <Icon name={focus ? "shrink" : "expand"} size={20} />
          <span className="focus-toggle__text">{focus ? "Показать меню" : "Доска в центре"}</span>
        </button>
      )}
      <nav className="nav" aria-label="Основное меню">
        <a className="nav__brand" href="#home" onClick={() => setFlyout(null)} aria-label="Dyanis Chess, на главную">
          <span className="nav__logo">
            <Icon name="crown" size={22} />
          </span>
          <span className="nav__name">Dyanis Chess</span>
        </a>

        <NavList items={NAV} page={menuId(page)} select={select} flyout={flyout} setFlyout={setFlyout} />
        <NavList items={[accountItem, SETTINGS_ITEM]} page={page} select={select} flyout={flyout} setFlyout={setFlyout} className="nav__list--bottom" />
        <AccountCard />

        <p className="nav__foot">
          Свой движок на Go
          <br />и нейросеть NNUE
        </p>
      </nav>

      <main className="main">
        {/* Only the visible page is mounted: the play page owns a Web Worker. */}
        {page === "home" && <HomePage />}
        {page === "review" && <ReviewPage />}
        {page === "analysis" && <AnalysisPage />}
        {page === "profile" && <ProfilePage />}
        {page === "online" && <OnlinePage />}
        {page === "bots" && <BotsPage />}
        {page === "variants" && <VariantsPage />}
        {page === "play" && <App />}
        {page === "puzzles" && (puzzleSection() === "rush" ? <RushPage /> : puzzleSection() === "duel" ? <DuelPage /> : <PuzzlesPage />)}
        {page === "openings" && <OpeningsPage />}
        {page === "lessons" && <LessonsPage />}
        {page === "coach" && <TrainerPage />}
        {page === "leaders" && <LeadersPage />}
        {page === "user" && <UserPage />}
        {page === "account" && <AccountPage />}
        {page === "settings" && <SettingsPage />}
      </main>
    </div>
  );
}

export default function Root() {
  return (
    <AuthProvider>
      <SettingsProvider>
        <Shell />
      </SettingsProvider>
    </AuthProvider>
  );
}

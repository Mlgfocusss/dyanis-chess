import { useEffect, useState } from "react";
import App from "./App.jsx";
import ReviewPage from "./review/ReviewPage.jsx";
import SettingsPage from "./settings/SettingsPage.jsx";
import AccountPage from "./auth/AccountPage.jsx";
import ProfilePage from "./profile/ProfilePage.jsx";
import OnlinePage from "./online/OnlinePage.jsx";
import BotsPage from "./bots/BotsPage.jsx";
import PuzzlesPage from "./puzzles/PuzzlesPage.jsx";
import AccountCard from "./auth/AccountCard.jsx";
import { AuthProvider, useAuth } from "./auth/AuthContext.jsx";
import { SettingsProvider } from "./settings/SettingsContext.jsx";
import Icon from "./ui/Icon.jsx";
import "./Root.css";

// The menu, in order of how often it is wanted: play first (with the three
// kinds of opponent), then puzzles, openings and lessons, then the review.
// Items marked soon are on the roadmap (WEB-PLAN.md) and shown disabled so the
// shape of the product is visible. The profile sits at the bottom left, in the
// account card (see auth/AccountCard.jsx).
const NAV = [
  {
    id: "play-menu",
    label: "Играть",
    icon: "play",
    children: [
      { id: "online", label: "Играть онлайн", icon: "online" },
      { id: "bots", label: "Играть с ботами", icon: "bots" },
      { id: "play", label: "Играть против движка", icon: "bot" },
    ],
  },
  { id: "puzzles", label: "Задачи", icon: "puzzles" },
  { id: "openings", label: "Дебюты", icon: "openings", soon: true },
  { id: "lessons", label: "Уроки", icon: "lessons", soon: true },
  { id: "review", label: "Разбор партии", icon: "review" },
  { id: "coach", label: "Тренер", icon: "coach", soon: true },
];
const SETTINGS_ITEM = { id: "settings", label: "Настройки", icon: "sliders" };
// Hides the menu and the panels so that only the board is left (see Root.css, .shell--focus).
const FOCUS_ITEM = { id: "focus", label: "Доска в центре", icon: "expand" };
const PAGES = ["review", "online", "bots", "play", "puzzles", "profile", "account", "settings"];
// Old addresses that moved.
const ALIASES = { games: "profile" };

// The page is kept in the URL hash (#play, #review/<id>) so a reload or a
// shared link lands on the same page; a router can replace this once
// there are pages with parameters.
function initialPage() {
  const raw = window.location.hash.slice(1).split("/")[0];
  const h = ALIASES[raw] || raw;
  return PAGES.includes(h) ? h : "review";
}

function Item({ it, active, onClick, open }) {
  return (
    <button
      type="button"
      className={"nav__item" + (active ? " nav__item--active" : "")}
      onClick={onClick}
      disabled={it.soon}
      aria-current={active && !it.children ? "page" : undefined}
      aria-expanded={it.children ? !!open : undefined}
    >
      <Icon name={it.icon} />
      <span className="nav__label">{it.label}</span>
      {it.soon && <span className="nav__soon">скоро</span>}
      {it.children && <Icon name="chevron" size={16} className={"nav__chev" + (open ? " nav__chev--open" : "")} />}
    </button>
  );
}

function NavList({ items, page, select, className = "", open, setOpen }) {
  return (
    <ul className={"nav__list " + className}>
      {items.map((it) => {
        const li = it.soon ? "nav__soon-item" : it.mobileOnly ? "nav__mobile-only" : undefined;
        if (!it.children) {
          return (
            <li key={it.id} className={li}>
              <Item it={it} active={page === it.id} onClick={() => select(it.id)} />
            </li>
          );
        }
        // A group: one button that opens a list of pages. It looks active while one of them is shown.
        const inside = it.children.some((c) => c.id === page);
        return (
          <li key={it.id} className="nav__group">
            <Item it={it} active={inside} open={open} onClick={() => setOpen(!open)} />
            {open && (
              <ul className="nav__sub">
                {it.children.map((c) => (
                  <li key={c.id} className={c.soon ? "nav__soon-item" : undefined}>
                    <Item it={c} active={page === c.id} onClick={() => select(c.id)} />
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
  if (page === "review" || page === "play" || page === "puzzles") return true;
  if (page === "online" || page === "bots") return !!window.location.hash.slice(1).split("/")[1];
  return false;
}

function Shell() {
  const { user, authError } = useAuth();
  // A failed Google sign-in lands on the account page, where the reason is shown.
  const [page, setPage] = useState(() => (authError ? "account" : initialPage()));

  // The play menu is open while one of its pages is shown, and can be opened by hand.
  const [playOpen, setPlayOpen] = useState(() => ["online", "bots", "play"].includes(initialPage()));
  const [focus, setFocus] = useState(false);
  const [, redraw] = useState(0); // the hash alone can change what the page offers (a game opens)
  function select(id) {
    if (id === "focus") {
      setFocus(true);
      return;
    }
    setFocus(false);
    setPage(id);
    window.location.hash = id;
    if (window.matchMedia("(max-width: 860px)").matches) setPlayOpen(false); // on the bar the list is a popup
  }

  // Links such as <a href="#review/<id>"> change the hash without our help.
  useEffect(() => {
    const onHash = () => {
      const p = initialPage();
      setPage(p);
      setFocus(false);
      redraw((n) => n + 1);
      if (p === "online" || p === "bots" || p === "play") setPlayOpen(true);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // The desktop menu has the profile / sign-in card at the bottom; the narrow
  // menu (a bar) gets this item instead.
  // Escape leaves the focus mode.
  useEffect(() => {
    if (!focus) return undefined;
    const onKey = (e) => e.key === "Escape" && setFocus(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [focus]);
  const canFocus = pageHasBoard(page);

  const accountItem = user
    ? { id: "profile", label: "Профиль", icon: "user", mobileOnly: true }
    : { id: "account", label: "Войти", icon: "user", mobileOnly: true };

  return (
    <div className={`shell${focus && canFocus ? " shell--focus" : ""}`}>
      {focus && canFocus && (
        <button type="button" className="focus-exit" onClick={() => setFocus(false)} title="Вернуть меню и панели (Esc)">
          <Icon name="shrink" size={18} /> Выйти из режима доски
        </button>
      )}
      <nav className="nav" aria-label="Основное меню">
        <div className="nav__brand">
          <span className="nav__logo">
            <Icon name="crown" size={22} />
          </span>
          <span className="nav__name">Dyanis Chess</span>
        </div>

        <NavList items={NAV} page={page} select={select} open={playOpen} setOpen={setPlayOpen} />
        <NavList items={canFocus ? [FOCUS_ITEM, accountItem, SETTINGS_ITEM] : [accountItem, SETTINGS_ITEM]} page={page} select={select} className="nav__list--bottom" />
        <AccountCard />

        <p className="nav__foot">
          Свой движок на Go
          <br />и нейросеть NNUE
        </p>
      </nav>

      <main className="main">
        {/* Only the visible page is mounted: the play page owns a Web Worker. */}
        {page === "review" && <ReviewPage />}
        {page === "profile" && <ProfilePage />}
        {page === "online" && <OnlinePage />}
        {page === "bots" && <BotsPage />}
        {page === "play" && <App />}
        {page === "puzzles" && <PuzzlesPage />}
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

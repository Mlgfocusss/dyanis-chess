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
import CommunityPage from "./community/CommunityPage.jsx";
import ForumPage from "./forum/ForumPage.jsx";
import Notices from "./community/Notices.jsx";
import AchievementToasts from "./achievements/AchievementToasts.jsx";
import { CommunityProvider, useCommunity } from "./community/CommunityContext.jsx";
import { badgeCount, parseCommunityHash } from "./community/logic.js";
import VariantsPage from "./variants/VariantsPage.jsx";
import { VARIANTS } from "./variants/registry.js";
import AccountCard from "./auth/AccountCard.jsx";
import LangSwitch from "./i18n/LangSwitch.jsx";
import { SettingsI18n } from "./i18n/I18nProvider.jsx";
import { AuthProvider, useAuth } from "./auth/AuthContext.jsx";
import { SettingsProvider, useSettings } from "./settings/SettingsContext.jsx";
import Icon from "./ui/Icon.jsx";
import { tx, useT } from "./i18n/index.js";
import "./Root.css";

// The menu, in order of how often it is wanted: play first (with the three
// kinds of opponent), then puzzles (rated ones, by theme, the puzzle of the
// day), openings and lessons, then the review.
// Items marked soon are on the roadmap (WEB-PLAN.md) and shown disabled so the
// shape of the product is visible. The profile sits at the bottom left, in the
// account card (see auth/AccountCard.jsx).
// On a phone the menu is a bottom bar with room for five buttons: the items without `more` stay on it,
// the rest (and the profile and the settings) open from its "More" button. `sheetTo` shows a group
// there as one link to that page instead of listing its pages.
const NAV = [
  { id: "home", labelKey: "menu.home", icon: "home" },
  {
    id: "play-menu",
    labelKey: "menu.play",
    icon: "play",
    children: [
      { id: "online", labelKey: "menu.online", icon: "online" },
      { id: "bots", labelKey: "menu.bots", icon: "bots" },
      { id: "play", labelKey: "menu.engine", icon: "bot" },
    ],
  },
  {
    // friends, messages, challenges and correspondence games; the leaderboard and the forum are here too
    id: "community-menu",
    labelKey: "menu.community",
    icon: "users",
    more: true,
    children: [
      { id: "community-friends", labelKey: "menu.friends", icon: "users" },
      { id: "community-messages", labelKey: "menu.messages", icon: "message" },
      { id: "community-challenges", labelKey: "menu.challenges", icon: "duel" },
      { id: "community-correspondence", labelKey: "menu.correspondence", icon: "clock" },
      { id: "forum", labelKey: "menu.forum", icon: "forum" },
      { id: "leaders", labelKey: "menu.leaders", icon: "trophy" },
    ],
  },
  {
    // "Другие шахматы": the pages of the variants come from their folders (web/src/variants/<id>/)
    id: "variants-menu",
    labelKey: "menu.variants",
    icon: "variants",
    more: true,
    sheetTo: "variants",
    children: [
      { id: "variants", labelKey: "menu.allVariants", icon: "list" },
      ...VARIANTS.map((v) => ({ id: `variant:${v.id}`, label: v.name, icon: v.icon })),
    ],
  },
  {
    id: "puzzles-menu",
    labelKey: "menu.puzzles",
    icon: "puzzles",
    children: [
      { id: "puzzles", labelKey: "menu.puzzlesRated", icon: "puzzles" },
      { id: "puzzles-themes", labelKey: "menu.puzzlesThemes", icon: "list" },
      { id: "puzzles-daily", labelKey: "menu.puzzlesDaily", icon: "trophy" },
      { id: "puzzles-rush", labelKey: "menu.rush", icon: "bolt" },
      { id: "puzzles-duel", labelKey: "menu.duel", icon: "duel" },
    ],
  },
  { id: "openings", labelKey: "menu.openings", icon: "openings", more: true },
  { id: "lessons", labelKey: "menu.lessons", icon: "lessons" },
  { id: "review", labelKey: "menu.review", icon: "review", more: true },
  { id: "analysis", labelKey: "menu.analysis", icon: "board", more: true },
  { id: "coach", labelKey: "menu.coach", icon: "coach", more: true },
];
const SETTINGS_ITEM = { id: "settings", labelKey: "menu.settings", icon: "sliders" };
const PAGES = ["home", "review", "analysis", "lessons", "coach", "online", "bots", "play", "puzzles", "variants", "openings", "leaders", "user", "community", "forum", "profile", "account", "settings"];
// The puzzle pages share one page and differ in the hash: [page, hash] of a menu item. So do the variants.
const SUB_PAGES = {
  ...Object.fromEntries(VARIANTS.map((v) => [`variant:${v.id}`, ["variants", `variants/${v.id}`]])),
  "community-friends": ["community", "community/friends"],
  "community-messages": ["community", "community/messages"],
  "community-challenges": ["community", "community/challenges"],
  "community-correspondence": ["community", "community/correspondence"],
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
  if (page === "community") return `community-${parseCommunityHash(window.location.hash).tab}`;
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

function Item({ it, active, onClick, open, badge, ...rest }) {
  const t = useT();
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
      <span className="nav__label">{it.labelKey ? t(it.labelKey) : tx(it.label)}</span>
      {it.soon && <span className="nav__soon">{t("common.soon")}</span>}
      {badge > 0 && (
        <span className="cbadge nav__badge" aria-label={t("menu.newCount", { n: badge })}>
          {badge > 99 ? "99+" : badge}
        </span>
      )}
      {it.children && <Icon name="chevron" size={16} className="nav__chev" />}
    </button>
  );
}

// A group of the menu is one button; its pages come out in a popup beside it while the pointer is
// on the button or the popup (on a touch screen: after a tap). flyout is { id, top, by } or null.
function NavList({ items, page, select, className = "", flyout, setFlyout, badges = {}, extra = null }) {
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
        const li = it.soon || it.noMobile ? "nav__soon-item" : it.mobileOnly ? "nav__mobile-only" : it.more ? "nav__more-item" : undefined;
        if (!it.children) {
          return (
            <li key={it.id} className={li}>
              <Item it={it} active={page === it.id} onClick={() => select(it.id)} badge={badges[it.id]} />
            </li>
          );
        }
        // It looks active while one of its pages is shown.
        const inside = it.children.some((c) => c.id === page);
        const open = flyout?.id === it.id;
        return (
          <li key={it.id} className={"nav__group" + (it.more ? " nav__more-item" : "")} onMouseLeave={hideSoon} onMouseEnter={(e) => show(it.id, e.currentTarget, "hover")} data-flyout-group>
            <Item
              it={it}
              active={inside}
              open={open}
              badge={badges[it.id]}
              onClick={(e) => (open && flyout.by === "click" ? setFlyout(null) : show(it.id, e.currentTarget.parentElement, "click"))}
            />
            {open && (
              <ul className="nav__sub" role="menu" style={{ "--fly-top": flyout.top + "px" }}>
                {it.children.map((c) => (
                  <li key={c.id} className={c.soon ? "nav__soon-item" : undefined} role="none">
                    <Item it={c} active={page === c.id} onClick={() => select(c.id)} role="menuitem" badge={badges[c.id]} />
                  </li>
                ))}
              </ul>
            )}
          </li>
        );
      })}
      {extra}
    </ul>
  );
}

// The "More" sheet of the phone menu: what does not fit on the bottom bar, in sections.
function MoreSheet({ items, select, onClose, page, badges }) {
  const t = useT();
  const label = (it) => (it.labelKey ? t(it.labelKey) : tx(it.label));
  const button = (it, to) => (
    <button key={it.id} type="button" className={"msheet__item" + (page === it.id ? " msheet__item--active" : "")} onClick={() => select(to || it.id)}>
      <Icon name={it.icon} size={20} />
      <span className="msheet__label">{label(it)}</span>
      {badges[it.id] > 0 && <span className="cbadge">{badges[it.id] > 99 ? "99+" : badges[it.id]}</span>}
    </button>
  );
  return (
    <div className="msheet" role="dialog" aria-label={t("menu.moreTitle")}>
      <button type="button" className="msheet__scrim" aria-label={t("menu.moreClose")} onClick={onClose} />
      <div className="msheet__panel">
        {items.map((it) =>
          it.children && !it.sheetTo ? (
            <section key={it.id} className="msheet__sec">
              <h3 className="msheet__title">
                {label(it)}
                {badges[it.id] > 0 && <span className="cbadge">{badges[it.id] > 99 ? "99+" : badges[it.id]}</span>}
              </h3>
              <div className="msheet__grid">{it.children.map((c) => button(c))}</div>
            </section>
          ) : (
            <div key={it.id} className="msheet__grid msheet__grid--one">
              {button(it, it.sheetTo)}
            </div>
          ),
        )}
      </div>
    </div>
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
  const t = useT();
  const { user, authError } = useAuth();
  const { settings, save } = useSettings();
  const { summary } = useCommunity();
  // The numbers on the menu: what waits for the person in the community.
  const badges = {
    "community-menu": badgeCount(summary),
    "community-friends": summary?.friendRequests || 0,
    "community-messages": summary?.messages || 0,
    "community-challenges": summary?.challenges || 0,
    "community-correspondence": summary?.yourTurn || 0,
  };
  // A failed Google sign-in lands on the account page, where the reason is shown.
  const [page, setPage] = useState(() => (authError ? "account" : initialPage()));

  const [flyout, setFlyout] = useState(null); // the group whose pages are shown in a popup
  const [moreOpen, setMoreOpen] = useState(false); // the "More" sheet of the phone menu
  const [, redraw] = useState(0); // the hash alone can change what the page offers (a game opens)
  const focus = settings.focusBoard;
  const setFocus = (v) => save({ ...settings, focusBoard: v });
  function select(id) {
    setFlyout(null);
    setMoreOpen(false);
    const [pg, hash] = SUB_PAGES[id] || [id, id];
    setPage(pg);
    window.location.hash = hash;
  }

  // Links such as <a href="#review/<id>"> change the hash without our help.
  useEffect(() => {
    const onHash = () => {
      setPage(initialPage());
      setFlyout(null);
      setMoreOpen(false);
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
    ? { id: "profile", labelKey: "menu.profile", icon: "user", mobileOnly: true }
    : { id: "account", labelKey: "menu.signIn", icon: "user", mobileOnly: true };

  // The phone menu: five buttons on the bar, the "More" one opens the rest.
  const moreItems = [...NAV.filter((it) => it.more), accountItem, SETTINGS_ITEM];
  const current = menuId(page);
  const moreActive = moreItems.some((it) => it.id === current || it.children?.some((c) => c.id === current));
  const moreButton = (
    <li className="nav__mobile-only">
      <button
        type="button"
        className={"nav__item" + (moreActive || moreOpen ? " nav__item--active" : "")}
        aria-expanded={moreOpen}
        aria-haspopup="dialog"
        onClick={() => {
          setFlyout(null);
          setMoreOpen((v) => !v);
        }}
      >
        <Icon name="more" />
        <span className="nav__label">{t("menu.more")}</span>
        {badges["community-menu"] > 0 && <span className="cbadge nav__badge">{badges["community-menu"] > 99 ? "99+" : badges["community-menu"]}</span>}
      </button>
    </li>
  );

  return (
    <div className={`shell${focused ? " shell--focus" : ""}`}>
      {canFocus && (
        <button
          type="button"
          className="focus-toggle"
          onClick={() => setFocus(!focus)}
          title={focus ? t("menu.focusOffTitle") : t("menu.focusOnTitle")}
          aria-label={focus ? t("menu.focusOffLabel") : t("menu.focusOnLabel")}
          aria-pressed={focus}
        >
          <Icon name={focus ? "shrink" : "expand"} size={20} />
          <span className="focus-toggle__text">{focus ? t("menu.focusOffText") : t("menu.focusOnText")}</span>
        </button>
      )}
      <nav className="nav" aria-label={t("menu.nav")}>
        <a className="nav__brand" href="#home" onClick={() => setFlyout(null)} aria-label={t("menu.toHome")}>
          <span className="nav__logo">
            <Icon name="crown" size={22} />
          </span>
          <span className="nav__name">Dyanis Chess</span>
        </a>

        <NavList items={NAV} page={current} select={select} flyout={flyout} setFlyout={setFlyout} badges={badges} extra={moreButton} />
        <NavList items={[accountItem, SETTINGS_ITEM]} page={page} select={select} flyout={flyout} setFlyout={setFlyout} className="nav__list--bottom" />
        <LangSwitch className="langsw--nav" />
        <AccountCard />

      </nav>
      {moreOpen && <MoreSheet items={moreItems} select={select} onClose={() => setMoreOpen(false)} page={current} badges={badges} />}

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
        {page === "community" && <CommunityPage />}
        {page === "forum" && <ForumPage />}
        {page === "user" && <UserPage />}
        {page === "account" && <AccountPage />}
        {page === "settings" && <SettingsPage />}
      </main>
      <Notices />
      <AchievementToasts />
    </div>
  );
}

export default function Root() {
  return (
    <AuthProvider>
      <SettingsProvider>
        <SettingsI18n>
          <CommunityProvider>
            <Shell />
          </CommunityProvider>
        </SettingsI18n>
      </SettingsProvider>
    </AuthProvider>
  );
}

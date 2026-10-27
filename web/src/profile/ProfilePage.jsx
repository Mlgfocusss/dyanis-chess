import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import GamesPage from "../games/GamesPage.jsx";
import BotAvatar from "../bots/BotAvatar.jsx";
import { useBots } from "../bots/api.js";
import { fetchRatings } from "../online/api.js";
import { categoryName, ratingText } from "../online/rating.js";
import Icon from "../ui/Icon.jsx";
import StatsTab, { useStats } from "./StatsTab.jsx";
import AchievementsTab from "../achievements/AchievementsTab.jsx";
import { streakBadge } from "./stats.js";
import { locale, useT } from "../i18n/index.js";
import "./profile.css";

// Tabs of the profile. Only "Партии" has content so far; the rest is
// planned (ratings, streaks, statistics) and says so.
const TABS = [
  { id: "overview", label: "profile.tabOverview" },
  { id: "games", label: "profile.tabGames" },
  { id: "stats", label: "profile.tabStats" },
  { id: "achievements", label: "achievements.title" },
];

function tabFromHash() {
  const t = window.location.hash.slice(1).split("/")[1];
  return TABS.some((x) => x.id === t) ? t : "games";
}

function memberSince(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  return d.toLocaleDateString(locale(), { day: "numeric", month: "long", year: "numeric" });
}

// Glicko-2 ratings of games against people, one per kind of game.
function RatingsOverview() {
  const t = useT();
  const [list, setList] = useState(null);
  useEffect(() => {
    fetchRatings().then(setList, () => setList([]));
  }, []);
  if (!list || list.length === 0) return null;
  return (
    <section className="card profile__ratings">
      <header>
        <h2>{t("profile.4")}</h2>
        <span className="profile__hint">{t("profile.5")}</span>
      </header>
      <ul className="profile__rlist">
        {list.map((r) => (
          <li key={r.category} className="profile__rating">
            <span className="profile__rname">{categoryName(r.category) || r.name}</span>
            <b className="profile__rvalue">{r.games ? ratingText(r.rating, r.provisional) : "—"}</b>
            <span className="profile__rsub">
              {r.games ? t("profile.6", { games: r.games, wins: r.wins, draws: r.draws, losses: r.losses }) : t("profile.notPlayedYet")}
            </span>
          </li>
        ))}
      </ul>
      <div className="room__row">
        <a className="btn btn--ghost" href="#online">
          {t("home.playOnline")}
        </a>
        <a className="btn btn--ghost" href="#puzzles/duel">
          {t("menu.duel")}
        </a>
      </div>
    </section>
  );
}

// The overview: for now, how the person has done against the bots.
function BotsOverview({ userId }) {
  const t = useT();
  const { data } = useBots(userId);
  if (!data) return null;
  const beaten = data.bots.filter((b) => data.progress[b.id]?.beaten);
  const played = data.bots.filter((b) => data.progress[b.id] && !data.progress[b.id].beaten);
  return (
    <section className="card profile__bots">
      <header>
        <h2>{t("avatar.bots")}</h2>
        <span className="profile__points">
          <Icon name="trophy" size={16} /> {data.points} {t("profile.11")} {beaten.length} {t("profile.12")} {data.bots.length}
        </span>
      </header>
      {beaten.length === 0 && played.length === 0 && (
        <p className="profile__hint">
          {t("profile.13")} <a href="#bots">{t("profile.14")}</a>
        </p>
      )}
      <ul className="profile__beaten">
        {beaten.map((b) => (
          <li key={b.id}>
            <BotAvatar bot={b} size={32} />
            <span className="profile__botname">
              {b.name} <small>{b.elo}</small>
            </span>
            <span className="profile__botpts">+{data.progress[b.id].points}</span>
          </li>
        ))}
      </ul>
      {beaten.length < data.bots.length && (
        <a className="btn btn--ghost" href="#bots">
          {t("menu.bots")}
        </a>
      )}
    </section>
  );
}

export default function ProfilePage() {
  const t = useT();
  const { status, user } = useAuth();
  const [tab, setTab] = useState(tabFromHash);
  // a link to another tab of the profile while the profile is open
  useEffect(() => {
    const onHash = () => setTab(tabFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // the numbers of the statistics tab; the streak tag in the head comes from the same answer
  const stats = useStats(user?.id);
  const badge = streakBadge(stats.stats?.streak);

  function select(id) {
    setTab(id);
    window.history.replaceState(null, "", `#profile/${id}`);
  }

  if (status === "loading") return <div className="page" />;

  // Not signed in: the games tab explains what to do.
  if (!user) return <GamesPage />;

  const since = memberSince(user.createdAt);
  return (
    <div className="page profile">
      <section className="card profile__head">
        <Avatar user={user} size={88} />
        <div className="profile__who">
          <div className="profile__namerow">
            <h1 className="profile__name">{user.name}</h1>
            {badge && (
              <span className="profile__streak" title={t("profile.16")}>
                <Icon name="flame" size={14} />
                {badge}
              </span>
            )}
          </div>
          <p className="profile__meta">
            {user.kind === "guest" ? t("auth.kindGuest") : t("auth.kindGoogle")}
            {since ? t("profile.19", { since }) : ""}
          </p>
        </div>
        <a className="btn btn--ghost profile__edit" href="#account">
          {t("auth.account")}
        </a>
      </section>

      <div className="tabs profile__tabs" role="tablist" aria-label={t("profile.21")}>
        {TABS.map((tb) => (
          <button
            key={tb.id}
            type="button"
            role="tab"
            aria-selected={tab === tb.id}
            className={`tabs__tab${tab === tb.id ? " tabs__tab--active" : ""}`}
            onClick={() => select(tb.id)}
          >
            {t(tb.label)}
          </button>
        ))}
      </div>

      {tab === "games" && <GamesPage embedded />}
      {tab === "overview" && (
        <>
          <RatingsOverview />
          <BotsOverview userId={user.id} />
        </>
      )}
      {tab === "stats" && <StatsTab {...stats} />}
      {tab === "achievements" && <AchievementsTab userId={user.id} />}
    </div>
  );
}

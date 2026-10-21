import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import GamesPage from "../games/GamesPage.jsx";
import BotAvatar from "../bots/BotAvatar.jsx";
import { useBots } from "../bots/api.js";
import { fetchRatings } from "../online/api.js";
import { ratingText } from "../online/rating.js";
import Icon from "../ui/Icon.jsx";
import StatsTab, { useStats } from "./StatsTab.jsx";
import { streakBadge } from "./stats.js";
import "./profile.css";

// Tabs of the profile. Only "Партии" has content so far; the rest is
// planned (ratings, streaks, statistics) and says so.
const TABS = [
  { id: "overview", label: "Обзор" },
  { id: "games", label: "Партии" },
  { id: "stats", label: "Статистика" },
];

function tabFromHash() {
  const t = window.location.hash.slice(1).split("/")[1];
  return TABS.some((x) => x.id === t) ? t : "games";
}

function memberSince(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  return d.toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" });
}

// Glicko-2 ratings of games against people, one per kind of game.
function RatingsOverview() {
  const [list, setList] = useState(null);
  useEffect(() => {
    fetchRatings().then(setList, () => setList([]));
  }, []);
  if (!list || list.length === 0) return null;
  return (
    <section className="card profile__ratings">
      <header>
        <h2>Рейтинги</h2>
        <span className="profile__hint">Партии с людьми, Glicko-2. «?» значит, что рейтинг ещё примерный.</span>
      </header>
      <ul className="profile__rlist">
        {list.map((r) => (
          <li key={r.category} className="profile__rating">
            <span className="profile__rname">{r.name}</span>
            <b className="profile__rvalue">{r.games ? ratingText(r.rating, r.provisional) : "—"}</b>
            <span className="profile__rsub">
              {r.games ? `партий ${r.games}: ${r.wins} / ${r.draws} / ${r.losses}` : "ещё не играли"}
            </span>
          </li>
        ))}
      </ul>
      <div className="room__row">
        <a className="btn btn--ghost" href="#online">
          Играть онлайн
        </a>
        <a className="btn btn--ghost" href="#puzzles/duel">
          Тактическая дуэль
        </a>
      </div>
    </section>
  );
}

// The overview: for now, how the person has done against the bots.
function BotsOverview({ userId }) {
  const { data } = useBots(userId);
  if (!data) return null;
  const beaten = data.bots.filter((b) => data.progress[b.id]?.beaten);
  const played = data.bots.filter((b) => data.progress[b.id] && !data.progress[b.id].beaten);
  return (
    <section className="card profile__bots">
      <header>
        <h2>Боты</h2>
        <span className="profile__points">
          <Icon name="trophy" size={16} /> {data.points} очков · побеждено {beaten.length} из {data.bots.length}
        </span>
      </header>
      {beaten.length === 0 && played.length === 0 && (
        <p className="profile__hint">
          Вы ещё не играли с ботами. За первую победу над каждым ботом начисляются очки. <a href="#bots">Выбрать бота</a>
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
          Играть с ботами
        </a>
      )}
    </section>
  );
}

export default function ProfilePage() {
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
              <span className="profile__streak" title="Дней подряд с задачами">
                <Icon name="flame" size={14} />
                {badge}
              </span>
            )}
          </div>
          <p className="profile__meta">
            {user.kind === "guest" ? "Гостевой аккаунт" : "Вход через Google"}
            {since ? ` · с нами с ${since}` : ""}
          </p>
        </div>
        <a className="btn btn--ghost profile__edit" href="#account">
          Аккаунт
        </a>
      </section>

      <div className="tabs profile__tabs" role="tablist" aria-label="Разделы профиля">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            className={`tabs__tab${tab === t.id ? " tabs__tab--active" : ""}`}
            onClick={() => select(t.id)}
          >
            {t.label}
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
    </div>
  );
}

import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import GamesPage from "../games/GamesPage.jsx";
import BotAvatar from "../bots/BotAvatar.jsx";
import { useBots } from "../bots/api.js";
import Icon from "../ui/Icon.jsx";
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
          <h1 className="profile__name">{user.name}</h1>
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
      {tab === "overview" && <BotsOverview userId={user.id} />}
      {tab === "stats" && (
        <section className="card profile__soon">
          <h2>Статистика</h2>
          <p>
            Здесь появятся рейтинги, серия дней, точность по партиям и динамика. Профиль будет доработан позже, а
            пока все ваши партии лежат во вкладке «Партии».
          </p>
        </section>
      )}
    </div>
  );
}

import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import { placeLine } from "../countries/countryList.js";
import Icon from "../ui/Icon.jsx";
import { fetchUser } from "./api.js";
import { botsLine, joinedText, kindLabel, leadersHash, profileRatings, userFromHash } from "./leaders.js";
import "./leaders.css";

// The public profile of another player (#user/<id>): who they are, ratings with their places in the
// tables, games and the points against the bots. Nothing private is sent by the server.
export default function UserPage() {
  const [target, setTarget] = useState(() => userFromHash(window.location.hash));
  useEffect(() => {
    const onHash = () => {
      const t = userFromHash(window.location.hash);
      setTarget((old) => (old.id === t.id && old.from === t.from && old.country === t.country ? old : t));
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return <UserView key={target.id} id={target.id} from={target.from} country={target.country} />;
}

function UserView({ id, from, country }) {
  const { user } = useAuth();
  const [state, setState] = useState({ profile: null, error: "", missing: false });

  useEffect(() => {
    if (!id) {
      setState({ profile: null, error: "", missing: true });
      return undefined;
    }
    const ctl = new AbortController();
    fetchUser(id, ctl.signal).then(
      (profile) => setState({ profile, error: "", missing: false }),
      (e) => {
        if (e.name === "AbortError") return;
        setState({ profile: null, error: e.status === 404 ? "" : e.message, missing: e.status === 404 });
      },
    );
    return () => ctl.abort();
  }, [id]);

  const { profile, error, missing } = state;
  const back = (
    <a className="btn btn--ghost uprof__back" href={leadersHash(from, country)}>
      <Icon name="back" size={18} />
      Назад к таблице
    </a>
  );

  if (missing || error) {
    return (
      <div className="page uprof">
        <div className="card uprof__empty">
          <h1>{missing ? "Игрок не найден" : "Не удалось загрузить профиль"}</h1>
          <p>{missing ? "Такого игрока нет: возможно, ссылка устарела." : error}</p>
          {back}
        </div>
      </div>
    );
  }
  if (!profile) {
    return (
      <div className="page uprof">
        <p className="leaders__note">Загрузка…</p>
      </div>
    );
  }

  const mine = user && user.id === profile.id;
  const since = joinedText(profile.createdAt);
  const place = placeLine(profile.location, profile.countryName);
  const ratings = profileRatings(profile);
  return (
    <div className="page uprof">
      <div className="uprof__bar">{back}</div>

      <section className="card uprof__head">
        <Avatar user={profile} size={72} />
        <div className="uprof__who">
          <h1 className="uprof__name" title={profile.name}>
            {profile.name}
            {mine && <span className="leaders__you">вы</span>}
          </h1>
          {profile.fullName && <p className="uprof__real">{profile.fullName}</p>}
          {place && (
            <p className="uprof__place-line">
              {profile.country && <span className="uprof__code" title={profile.countryName}>{profile.country}</span>}
              {place}
            </p>
          )}
          <p className="uprof__meta">
            {kindLabel(profile.kind)}
            {since && ` · на сайте с ${since}`}
          </p>
        </div>
        {mine && (
          <a className="btn btn--ghost" href="#profile">
            Мой профиль
          </a>
        )}
      </section>

      {profile.about && (
        <section className="card uprof__card">
          <header>
            <h2>Обо мне</h2>
          </header>
          {/* plain text: React escapes it, the line breaks are kept by CSS */}
          <p className="uprof__about">{profile.about}</p>
        </section>
      )}

      <section className="card uprof__card">
        <header>
          <h2>Рейтинги</h2>
          <span className="uprof__hint">Партии между людьми, Glicko-2. «?» значит, что рейтинг ещё примерный.</span>
        </header>
        <ul className="uprof__ratings">
          {ratings.map((r) => (
            <li key={r.category} className="uprof__rating">
              <span className="uprof__rname">{r.name}</span>
              <b className="uprof__rvalue">{r.value}</b>
              <span className="uprof__rsub">{r.record}</span>
              {r.place > 0 ? (
                <a className="uprof__place" href={leadersHash(r.category)}>
                  <Icon name="trophy" size={14} />
                  {r.placeLine}
                </a>
              ) : (
                <span className="uprof__rsub">{r.games ? "вне таблицы" : " "}</span>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="card uprof__card">
        <header>
          <h2>Партии и боты</h2>
        </header>
        <dl className="uprof__stats">
          <div>
            <dt>Партий сыграно</dt>
            <dd>{profile.games}</dd>
          </div>
          <div>
            <dt>Очки у ботов</dt>
            <dd className="uprof__points">
              <Icon name="star" size={18} />
              {profile.bots?.points || 0}
            </dd>
          </div>
          <div>
            <dt>Боты</dt>
            <dd>{botsLine(profile.bots) || "—"}</dd>
          </div>
        </dl>
      </section>
    </div>
  );
}

import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import CountryPicker from "../countries/CountryPicker.jsx";
import useCountries from "../countries/useCountries.js";
import Icon from "../ui/Icon.jsx";
import { fetchLeaderboard } from "./api.js";
import {
  CATEGORIES,
  MIN_GAMES,
  categoryFromHash,
  categoryName,
  countryCell,
  countryFromHash,
  emptyCountryLine,
  emptyLine,
  isMine,
  leadersHash,
  medal,
  placeText,
  playHash,
  rankRows,
  userHash,
} from "./leaders.js";
import "./leaders.css";

// The leaderboard: tabs of the rating categories, the best players of the chosen one.
export default function LeadersPage() {
  const { user } = useAuth();
  const [category, setCategory] = useState(() => categoryFromHash(window.location.hash));
  const [country, setCountry] = useState(() => countryFromHash(window.location.hash));
  const { countries } = useCountries();
  const [state, setState] = useState({ rows: null, error: "" }); // rows null while loading

  // The tabs are links: the address carries the category, so a reload or a shared link lands on it.
  useEffect(() => {
    const onHash = () => {
      setCategory(categoryFromHash(window.location.hash));
      setCountry(countryFromHash(window.location.hash));
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    const ctl = new AbortController();
    setState({ rows: null, error: "" });
    fetchLeaderboard(category, country, ctl.signal).then(
      (rows) => setState({ rows: rankRows(rows), error: "" }),
      (e) => {
        if (e.name !== "AbortError") setState({ rows: [], error: e.message });
      },
    );
    return () => ctl.abort();
  }, [category, country]);

  const { rows, error } = state;
  return (
    <div className="page leaders">
      <header className="page__head">
        <h1 className="page__title">Таблица лидеров</h1>
        <p className="page__sub">Лучшие игроки сайта по рейтингу Glicko-2: партии между людьми и тактическая дуэль.</p>
      </header>

      <div className="leaders__tabs" role="tablist" aria-label="Категория рейтинга">
        {CATEGORIES.map((c) => (
          <a
            key={c}
            role="tab"
            href={leadersHash(c, country)}
            aria-selected={c === category}
            className={"leaders__tab" + (c === category ? " leaders__tab--active" : "")}
          >
            {categoryName(c)}
          </a>
        ))}
      </div>

      <div className="leaders__filter">
        <label className="leaders__filterlabel" htmlFor="leaders-country">
          Страна
        </label>
        <div className="leaders__filterpick">
          <CountryPicker
            id="leaders-country"
            value={country}
            countries={countries}
            emptyLabel="Все страны"
            onChange={(code) => {
              window.location.hash = leadersHash(category, code);
            }}
          />
        </div>
      </div>

      {error && <div className="banner banner--error">{error}</div>}

      <section className="card leaders__card" aria-live="polite" aria-busy={rows === null}>
        {rows === null && <p className="leaders__note">Загрузка…</p>}
        {rows && rows.length === 0 && !error && (
          <div className="leaders__empty">
            <Icon name="trophy" size={32} />
            <h2>Пока никто не набрал рейтинг</h2>
            <p>{country ? emptyCountryLine(countries, country) : emptyLine(category)}</p>
            {country ? (
              <a className="btn" href={leadersHash(category)}>
                Все страны
              </a>
            ) : (
              <a className="btn btn--primary" href={playHash(category)}>
                {category === "duel" ? "Тактическая дуэль" : "Играть онлайн"}
              </a>
            )}
          </div>
        )}
        {rows && rows.length > 0 && (
          <table className="leaders__table">
            <thead>
              <tr>
                <th className="leaders__place" scope="col">
                  №
                </th>
                <th scope="col">Игрок</th>
                <th className="leaders__country" scope="col">
                  Страна
                </th>
                <th className="leaders__num" scope="col">
                  Рейтинг
                </th>
                <th className="leaders__num leaders__games" scope="col">
                  Партий
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className={isMine(r, user) ? "leaders__row leaders__row--me" : "leaders__row"}>
                  <td className={"leaders__place" + (medal(r.place) ? ` leaders__place--${medal(r.place)}` : "")}>{placeText(r.place)}</td>
                  <td className="leaders__who">
                    <a className="leaders__link" href={userHash(r.id, category, country)} title={r.name}>
                      <Avatar user={r} size={28} />
                      <span className="leaders__nameblock">
                        <span className="leaders__name">{r.name}</span>
                        {r.country && <span className="leaders__subcountry">{countryCell(countries, r.country)}</span>}
                      </span>
                    </a>
                    {isMine(r, user) && <span className="leaders__you">вы</span>}
                  </td>
                  <td className="leaders__country" title={countryCell(countries, r.country)}>
                    {countryCell(countries, r.country)}
                  </td>
                  <td className="leaders__num leaders__rating">{r.rating}</td>
                  <td className="leaders__num leaders__games">{r.games}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <p className="leaders__foot">
        В таблицу попадают игроки, сыгравшие не меньше {MIN_GAMES} партий в категории, с уже не примерным рейтингом (без «?»). Рейтинг игрока, который
        долго не играет, снова становится примерным, и он на время уходит из таблицы.
      </p>
    </div>
  );
}

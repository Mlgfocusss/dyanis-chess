import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import { choiceToValue } from "../auth/avatarPresets.js";
import CountryPicker from "../countries/CountryPicker.jsx";
import { countryName, placeLine } from "../countries/countryList.js";
import useCountries from "../countries/useCountries.js";
import Icon from "../ui/Icon.jsx";
import { joinedText } from "../leaders/leaders.js";
import AvatarPicker from "./AvatarPicker.jsx";
import SignInOffer from "./SignInOffer.jsx";
import { NAME_MAX, nameLength } from "./nameRules.js";
import { ABOUT_MAX, LOCATION_MAX, PERSON_MAX, aboutCounter, checkProfile, diffProfile, fullName, savedValues, textLength } from "./profileFields.js";

// "Профиль": the open profile in the manner of chess.com: the avatar, the display name, the first and last
// name, the country, the place and the "about me" text. All of it is public (the player's page and the
// leaderboard); the e-mail and the settings never are. The server has the last word on every rule (and
// limits saves: the name once a minute, the rest once in 5 seconds).
export default function ProfilePanel() {
  const { status, user, updateProfile } = useAuth();
  const { countries, ready: countriesReady, error: countriesError } = useCountries();
  const [edits, setEdits] = useState({}); // field -> what was typed; a field not here shows the saved value
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);

  if (status !== "ready" || !user) {
    return (
      <>
        <h2 className="settings__h">Профиль</h2>
        <SignInOffer />
      </>
    );
  }

  const base = savedValues(user);
  const values = { ...base, ...edits };
  const check = checkProfile(values);
  const patch = diffProfile(base, check.values);
  const changed = Object.keys(patch).length > 0;
  // The rules are shown once the person has touched the field, not on the untouched form.
  const problem = (f) => (edits[f] !== undefined ? check.errors[f] || "" : "");

  function edit(field, value) {
    setEdits((e) => ({ ...e, [field]: value }));
    setError("");
    setSaved(false);
  }

  async function onSubmit(e) {
    e.preventDefault();
    if (busy || !changed || !check.valid) return;
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      await updateProfile(patch);
      setEdits({});
      setSaved(true);
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  function reset() {
    setEdits({});
    setError("");
    setSaved(false);
  }

  const shown = check.values;
  const preview = {
    name: (check.errors.name ? base.name : shown.name) || base.name,
    avatarUrl: choiceToValue(shown.avatar, user.googleAvatarUrl),
  };
  const place = placeLine(shown.location, countryName(countries, shown.country));
  const since = joinedText(user.createdAt);

  return (
    <>
      <h2 className="settings__h">Профиль</h2>
      <p className="settings__sub">Ваша открытая страница: так вас видят соперники, в таблице лидеров и в списке партий.</p>
      <p className="profnote">
        <Icon name="user" size={18} />
        <span>
          Аватар, имя, страну, местоположение и «Обо мне» видят все, кто откроет вашу страницу игрока. Почту и настройки не видит никто.
        </span>
      </p>

      <div className="pcard pcard--open">
        <Avatar user={preview} size={72} />
        <div className="pcard__who">
          <div className="pcard__name">{preview.name}</div>
          {fullName(shown.firstName, shown.lastName) && <div className="pcard__real">{fullName(shown.firstName, shown.lastName)}</div>}
          {place && <div className="pcard__meta">{place}</div>}
          <div className="pcard__meta">
            {user.kind === "guest" ? "Гостевой аккаунт" : "Вход через Google"}
            {since && ` · на сайте с ${since}`}
          </div>
        </div>
      </div>

      <form className="profform" onSubmit={onSubmit} noValidate>
        <section className="profform__block">
          <h3 className="profform__h" id="profile-avatar-h">
            Аватар
          </h3>
          <AvatarPicker
            choice={shown.avatar}
            onChange={(v) => edit("avatar", v)}
            googleAvatarUrl={user.googleAvatarUrl}
            name={preview.name}
          />
          <p className="srow__hint profform__hint">
            {user.googleAvatarUrl ? "Можно оставить фото Google, выбрать готовый аватар или букву имени." : "Готовый аватар или буква имени. Фото Google появится, если войти через Google."}
          </p>
        </section>

        <section className="profform__block">
          <Field
            id="profile-name"
            label="Отображаемое имя"
            error={problem("name")}
            count={`${nameLength(shown.name)}/${NAME_MAX}`}
            bad={nameLength(shown.name) > NAME_MAX}
            hint={`От 2 до ${NAME_MAX} символов: буквы, цифры, пробел, дефис, подчёркивание и точка. Имена могут повторяться. Менять имя можно не чаще раза в минуту; в уже начатых партиях остаётся прежнее.`}
          >
            <input
              id="profile-name"
              className={`field${problem("name") ? " field--bad" : ""}`}
              value={values.name}
              autoComplete="nickname"
              spellCheck={false}
              aria-invalid={problem("name") ? "true" : undefined}
              aria-describedby="profile-name-hint"
              onChange={(e) => edit("name", e.target.value)}
            />
          </Field>

          <div className="profform__two">
            <Field
              id="profile-first"
              label="Имя"
              error={problem("firstName")}
              count={`${textLength(shown.firstName)}/${PERSON_MAX}`}
              bad={textLength(shown.firstName) > PERSON_MAX}
            >
              <input
                id="profile-first"
                className={`field${problem("firstName") ? " field--bad" : ""}`}
                value={values.firstName}
                autoComplete="given-name"
                aria-invalid={problem("firstName") ? "true" : undefined}
                onChange={(e) => edit("firstName", e.target.value)}
              />
            </Field>
            <Field
              id="profile-last"
              label="Фамилия"
              error={problem("lastName")}
              count={`${textLength(shown.lastName)}/${PERSON_MAX}`}
              bad={textLength(shown.lastName) > PERSON_MAX}
            >
              <input
                id="profile-last"
                className={`field${problem("lastName") ? " field--bad" : ""}`}
                value={values.lastName}
                autoComplete="family-name"
                aria-invalid={problem("lastName") ? "true" : undefined}
                onChange={(e) => edit("lastName", e.target.value)}
              />
            </Field>
          </div>

          <div className="profform__two">
            <Field
              id="profile-country"
              label="Страна"
              hint={countriesError ? "Список стран не загрузился. Обновите страницу." : "Нужна для таблицы лидеров по странам."}
            >
              <CountryPicker
                id="profile-country"
                value={values.country}
                countries={countries}
                disabled={!countriesReady}
                onChange={(code) => edit("country", code)}
              />
            </Field>
            <Field
              id="profile-location"
              label="Местоположение"
              error={problem("location")}
              count={`${textLength(shown.location)}/${LOCATION_MAX}`}
              bad={textLength(shown.location) > LOCATION_MAX}
            >
              <input
                id="profile-location"
                className={`field${problem("location") ? " field--bad" : ""}`}
                value={values.location}
                placeholder="Город"
                autoComplete="off"
                aria-invalid={problem("location") ? "true" : undefined}
                onChange={(e) => edit("location", e.target.value)}
              />
            </Field>
          </div>

          <Field
            id="profile-about"
            label="Обо мне"
            error={problem("about")}
            count={aboutCounter(values.about)}
            bad={textLength(shown.about) > ABOUT_MAX}
            hint="Несколько слов о себе. Только текст: ссылки и разметка не работают."
          >
            <textarea
              id="profile-about"
              className={`field profform__about${problem("about") ? " field--bad" : ""}`}
              rows={5}
              value={values.about}
              aria-invalid={problem("about") ? "true" : undefined}
              aria-describedby="profile-about-hint"
              onChange={(e) => edit("about", e.target.value)}
            />
          </Field>
        </section>

        {error && (
          <p className="nameform__error" role="alert">
            {error}
          </p>
        )}
        <div className="savebar">
          <button type="button" className="btn" disabled={!changed || busy} onClick={reset}>
            Отмена
          </button>
          <button type="submit" className="btn btn--primary" disabled={!changed || !check.valid || busy}>
            Сохранить
          </button>
          {saved && !changed && (
            <span className="savebar__ok" role="status">
              Профиль сохранён
            </span>
          )}
        </div>
      </form>
    </>
  );
}

// A labelled field: the label, the control, a counter beside the label, a hint and the message about a problem.
function Field({ id, label, count, bad, hint, error, children }) {
  return (
    <div className="pfield">
      <div className="pfield__head">
        <label className="nameform__label" htmlFor={id}>
          {label}
        </label>
        {count && (
          <span className={`nameform__count${bad ? " nameform__count--bad" : ""}`} aria-live="polite">
            {count}
          </span>
        )}
      </div>
      {children}
      {hint && (
        <p id={`${id}-hint`} className="srow__hint pfield__hint">
          {hint}
        </p>
      )}
      {error && (
        <p className="nameform__error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

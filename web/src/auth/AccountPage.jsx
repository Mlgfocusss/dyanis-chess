import { useState } from "react";
import { useAuth } from "./AuthContext.jsx";
import { GOOGLE_LOGIN_URL } from "./api.js";
import Avatar, { GoogleLogo } from "./Avatar.jsx";
import "./auth.css";

function GoogleButton({ enabled, children }) {
  if (!enabled) {
    return (
      <button type="button" className="btn btn--block" disabled title="Вход через Google не настроен на сервере">
        <GoogleLogo /> {children}
      </button>
    );
  }
  // A plain link: the browser leaves for Google and comes back signed in.
  return (
    <a className="btn btn--block btn--google" href={GOOGLE_LOGIN_URL}>
      <GoogleLogo /> {children}
    </a>
  );
}

export default function AccountPage() {
  const { status, user, google, authError, guest, logout } = useAuth();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmOut, setConfirmOut] = useState(false);

  async function run(fn) {
    setError("");
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  if (status === "loading") return <div className="page" />;

  if (status === "unavailable") {
    return (
      <div className="page account">
        <header className="page__head">
          <h1 className="page__title">Аккаунт</h1>
        </header>
        <div className="banner banner--error">
          Аккаунты сейчас недоступны: сервер запущен без базы данных. Разбор и игра с ботом работают как раньше.
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="page account">
        <header className="page__head">
          <h1 className="page__title">Вход</h1>
          <p className="page__sub">Чтобы ваши партии и настройки доски сохранялись и открывались на любом устройстве.</p>
        </header>
        {authError && <div className="banner banner--error">{authError}</div>}
        {error && <div className="banner banner--error">{error}</div>}
        <div className="account__options">
          <section className="card account__option">
            <h2>Через Google</h2>
            <p>Партии и настройки привязаны к вашему аккаунту. Пароль мы не получаем и не храним.</p>
            <GoogleButton enabled={google}>Войти через Google</GoogleButton>
          </section>
          <section className="card account__option">
            <h2>Как гость</h2>
            <p>Без регистрации, в один клик. Партии сохранятся в этом браузере; позже гостя можно привязать к Google.</p>
            <button type="button" className="btn btn--block" disabled={busy} onClick={() => run(guest)}>
              Продолжить как гость
            </button>
          </section>
        </div>
      </div>
    );
  }

  const isGuest = user.kind === "guest";
  return (
    <div className="page account">
      <header className="page__head">
        <h1 className="page__title">Аккаунт</h1>
      </header>
      {authError && <div className="banner banner--error">{authError}</div>}
      {error && <div className="banner banner--error">{error}</div>}
      <section className="card account__me">
        <Avatar user={user} size={56} />
        <div className="account__who">
          <div className="account__name">{user.name}</div>
          <div className="account__meta">
            {isGuest ? "Гостевой аккаунт" : "Вход через Google"}
            {user.email ? ` · ${user.email}` : ""}
          </div>
        </div>
      </section>

      {isGuest && (
        <section className="card account__option account__upgrade">
          <h2>Не потеряйте партии</h2>
          <p>
            Гость живёт, пока в браузере есть его cookie: очистите данные сайта или зайдите с другого устройства, и
            всё пропадёт. Войдите через Google, и этот аккаунт (с партиями и настройками) станет вашим постоянным.
          </p>
          <GoogleButton enabled={google}>Привязать Google</GoogleButton>
        </section>
      )}

      <div className="account__actions">
        {isGuest && confirmOut ? (
          <>
            <span className="account__warn">Гостя после выхода не вернуть. Выйти?</span>
            <button type="button" className="btn btn--danger" disabled={busy} onClick={() => run(logout)}>
              Да, выйти
            </button>
            <button type="button" className="btn btn--ghost" onClick={() => setConfirmOut(false)}>
              Отмена
            </button>
          </>
        ) : (
          <button
            type="button"
            className="btn btn--ghost"
            disabled={busy}
            onClick={() => (isGuest ? setConfirmOut(true) : run(logout))}
          >
            Выйти
          </button>
        )}
      </div>
    </div>
  );
}

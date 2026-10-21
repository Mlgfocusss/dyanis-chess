import { useState } from "react";
import { useAuth } from "./AuthContext.jsx";
import Avatar from "./Avatar.jsx";
import { GoogleButton, GoogleUpgrade, SignOut } from "./AccountParts.jsx";
import "./auth.css";

export default function AccountPage() {
  const { status, user, google, authError, guest } = useAuth();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

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

      {isGuest && <GoogleUpgrade />}

      <div className="account__actions">
        <a className="btn" href="#settings/profile">
          Изменить имя
        </a>
        <a className="btn" href="#settings/account">
          Учётная запись и удаление
        </a>
        <SignOut />
      </div>
    </div>
  );
}

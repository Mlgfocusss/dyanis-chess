import { useState } from "react";
import Icon from "../ui/Icon.jsx";
import { useAuth } from "./AuthContext.jsx";
import { GOOGLE_LOGIN_URL } from "./api.js";
import { GoogleLogo } from "./Avatar.jsx";
import "./auth.css";

// Pieces of the account screens shared by the account page (#account) and the
// "Учётная запись" section of the settings.

// Sign in with Google: a plain link, the browser leaves for Google and comes back signed in.
export function GoogleButton({ enabled, children }) {
  if (!enabled) {
    return (
      <button type="button" className="btn btn--block" disabled title="Вход через Google не настроен на сервере">
        <GoogleLogo /> {children}
      </button>
    );
  }
  return (
    <a className="btn btn--block btn--google" href={GOOGLE_LOGIN_URL}>
      <GoogleLogo /> {children}
    </a>
  );
}

// The offer to a guest: link Google, keep the games.
export function GoogleUpgrade() {
  const { google } = useAuth();
  return (
    <section className="card account__option account__upgrade">
      <h2>Не потеряйте партии</h2>
      <p>
        Гость живёт, пока в браузере есть его cookie: очистите данные сайта или зайдите с другого устройства, и всё
        пропадёт. Войдите через Google, и этот аккаунт (с партиями и настройками) станет вашим постоянным.
      </p>
      <GoogleButton enabled={google}>Привязать Google</GoogleButton>
    </section>
  );
}

// "Выйти"; a guest cannot come back, so it asks first.
export function SignOut({ label = "Выйти", icon = false }) {
  const { user, logout } = useAuth();
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const isGuest = user?.kind === "guest";

  async function run() {
    setBusy(true);
    setError("");
    try {
      await logout();
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <div className="account__actions">
      {isGuest && confirm ? (
        <>
          <span className="account__warn">Гостя после выхода не вернуть. Выйти?</span>
          <button type="button" className="btn btn--danger" disabled={busy} onClick={run}>
            Да, выйти
          </button>
          <button type="button" className="btn btn--ghost" onClick={() => setConfirm(false)}>
            Отмена
          </button>
        </>
      ) : (
        <button type="button" className="btn btn--ghost" disabled={busy} onClick={() => (isGuest ? setConfirm(true) : run())}>
          {icon && <Icon name="logout" size={18} />}
          {label}
        </button>
      )}
      {error && <span className="account__warn account__warn--error">{error}</span>}
    </div>
  );
}

// "12 марта 2026", or "" for a missing or broken date.
export function formatDate(iso) {
  const d = new Date(iso);
  if (!iso || Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" });
}

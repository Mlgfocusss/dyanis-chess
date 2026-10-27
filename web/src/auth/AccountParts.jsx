import { useState } from "react";
import Icon from "../ui/Icon.jsx";
import { useAuth } from "./AuthContext.jsx";
import { GOOGLE_LOGIN_URL } from "./api.js";
import { GoogleLogo } from "./Avatar.jsx";
import { locale, useT } from "../i18n/index.js";
import "./auth.css";

// Pieces of the account screens shared by the account page (#account) and the
// "Учётная запись" section of the settings.

// Sign in with Google: a plain link, the browser leaves for Google and comes back signed in.
export function GoogleButton({ enabled, children }) {
  const t = useT();
  if (!enabled) {
    return (
      <button type="button" className="btn btn--block" disabled title={t("auth.googleNotConfigured")}>
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
  const t = useT();
  const { google } = useAuth();
  return (
    <section className="card account__option account__upgrade">
      <h2>{t("auth.keepGames")}</h2>
      <p>
        {t("auth.keepGamesText")}
      </p>
      <GoogleButton enabled={google}>{t("auth.linkGoogle")}</GoogleButton>
    </section>
  );
}

// "Выйти"; a guest cannot come back, so it asks first.
export function SignOut({ label, icon = false }) {
  const t = useT();
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
          <span className="account__warn">{t("auth.guestSignOutWarn")}</span>
          <button type="button" className="btn btn--danger" disabled={busy} onClick={run}>
            {t("auth.yesSignOut")}
          </button>
          <button type="button" className="btn btn--ghost" onClick={() => setConfirm(false)}>
            {t("common.cancel")}
          </button>
        </>
      ) : (
        <button type="button" className="btn btn--ghost" disabled={busy} onClick={() => (isGuest ? setConfirm(true) : run())}>
          {icon && <Icon name="logout" size={18} />}
          {label ?? t("auth.signOut")}
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
  return d.toLocaleDateString(locale(), { day: "numeric", month: "long", year: "numeric" });
}

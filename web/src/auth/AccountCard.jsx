import { useState } from "react";
import { useAuth } from "./AuthContext.jsx";
import { GOOGLE_LOGIN_URL } from "./api.js";
import Avatar, { GoogleLogo } from "./Avatar.jsx";
import { useT } from "../i18n/index.js";

// Bottom of the menu: offers to sign in (Google or as a guest) to someone
// who has not, and shows who is signed in otherwise (a click opens the
// account page, where one can sign out). Hidden on narrow screens, where the
// menu has a plain "Войти" item instead.
export default function AccountCard() {
  const t = useT();
  const { status, user, google, guest } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (status !== "ready") return null; // loading, or the server has no accounts

  if (user) {
    return (
      <a className="acard acard--user" href="#profile" title={t("auth.openProfile")}>
        <Avatar user={user} size={36} />
        <span className="acard__who">
          <span className="acard__name">{user.name}</span>
          <span className="acard__kind">{t("menu.profile")}{user.kind === "guest" ? t("auth.guestSuffix") : ""}</span>
        </span>
      </a>
    );
  }

  async function asGuest() {
    setBusy(true);
    setError("");
    try {
      await guest();
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <div className="acard">
      <p className="acard__title">{t("auth.saveGames")}</p>
      <p className="acard__text">{t("auth.saveGamesText")}</p>
      {google ? (
        <a className="btn btn--google btn--block" href={GOOGLE_LOGIN_URL}>
          <GoogleLogo size={16} /> {t("auth.signInGoogle")}
        </a>
      ) : null}
      <button type="button" className="btn btn--block" onClick={asGuest} disabled={busy}>
        {t("auth.asGuest")}
      </button>
      {error && <p className="acard__error">{error}</p>}
    </div>
  );
}

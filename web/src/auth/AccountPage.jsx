import { useState } from "react";
import { useAuth } from "./AuthContext.jsx";
import Avatar from "./Avatar.jsx";
import { GoogleButton, GoogleUpgrade, SignOut } from "./AccountParts.jsx";
import { useT } from "../i18n/index.js";
import "./auth.css";

export default function AccountPage() {
  const t = useT();
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
          <h1 className="page__title">{t("auth.account")}</h1>
        </header>
        <div className="banner banner--error">
          {t("auth.unavailable")}
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="page account">
        <header className="page__head">
          <h1 className="page__title">{t("auth.signInTitle")}</h1>
          <p className="page__sub">{t("auth.signInSub")}</p>
        </header>
        {authError && <div className="banner banner--error">{t(authError)}</div>}
        {error && <div className="banner banner--error">{error}</div>}
        <div className="account__options">
          <section className="card account__option">
            <h2>{t("auth.viaGoogle")}</h2>
            <p>{t("auth.viaGoogleText")}</p>
            <GoogleButton enabled={google}>{t("auth.signInGoogle")}</GoogleButton>
          </section>
          <section className="card account__option">
            <h2>{t("auth.guestTitle")}</h2>
            <p>{t("auth.guestText")}</p>
            <button type="button" className="btn btn--block" disabled={busy} onClick={() => run(guest)}>
              {t("auth.asGuest")}
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
        <h1 className="page__title">{t("auth.account")}</h1>
      </header>
      {authError && <div className="banner banner--error">{t(authError)}</div>}
      {error && <div className="banner banner--error">{error}</div>}
      <section className="card account__me">
        <Avatar user={user} size={56} />
        <div className="account__who">
          <div className="account__name">{user.name}</div>
          <div className="account__meta">
            {isGuest ? t("auth.kindGuest") : t("auth.kindGoogle")}
            {user.email ? ` · ${user.email}` : ""}
          </div>
        </div>
      </section>

      {isGuest && <GoogleUpgrade />}

      <div className="account__actions">
        <a className="btn" href="#settings/profile">
          {t("auth.changeName")}
        </a>
        <a className="btn" href="#settings/account">
          {t("auth.accountAndDelete")}
        </a>
        <SignOut />
      </div>
    </div>
  );
}

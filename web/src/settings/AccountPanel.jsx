import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import { GoogleUpgrade, SignOut, formatDate } from "../auth/AccountParts.jsx";
import { Row } from "./controls.jsx";
import SignInOffer from "./SignInOffer.jsx";
import { confirmsDelete } from "./nameRules.js";
import { useT } from "../i18n/index.js";

// "Учётная запись": what kind of account this is, signing out, deleting it for good.
export default function AccountPanel() {
  const t = useT();
  const { status, user } = useAuth();
  const [deleted, setDeleted] = useState(false);

  if (status !== "ready" || !user) {
    return (
      <>
        <h2 className="settings__h">{t("settings.secAccount")}</h2>
        {deleted && (
          <div className="savebar__ok settings__done" role="status">
            {t("accountPanel.2")}
          </div>
        )}
        <SignInOffer />
      </>
    );
  }

  const isGuest = user.kind === "guest";
  const since = formatDate(user.createdAt);
  return (
    <>
      <h2 className="settings__h">{t("settings.secAccount")}</h2>
      <p className="settings__sub">{t("accountPanel.3")}</p>

      <div className="pcard">
        <Avatar user={user} size={64} />
        <div className="pcard__who">
          <div className="pcard__name">{user.name}</div>
          <div className="pcard__meta">
            <a href="#settings/profile">{t("auth.changeName")}</a>
          </div>
        </div>
      </div>

      <dl className="facts">
        <div className="facts__row">
          <dt>{t("accountPanel.5")}</dt>
          <dd>{isGuest ? t("accountPanel.6") : "Google"}</dd>
        </div>
        {!isGuest && user.email && (
          <div className="facts__row">
            <dt>{t("accountPanel.7")}</dt>
            <dd>{user.email}</dd>
          </div>
        )}
        {since && (
          <div className="facts__row">
            <dt>{t("accountPanel.8")}</dt>
            <dd>{since}</dd>
          </div>
        )}
      </dl>

      {isGuest && (
        <div className="settings__upgrade">
          <GoogleUpgrade />
        </div>
      )}

      <Row title={t("auth.signOut")} hint={isGuest ? t("accountPanel.10") : t("accountPanel.11")}>
        <SignOut icon />
      </Row>

      <Row
        title={t("accountPanel.12")}
        hint={t("accountPanel.13")}
      >
        <DeleteAccount user={user} onDone={() => setDeleted(true)} />
      </Row>
    </>
  );
}

function DeleteAccount({ user, onDone }) {
  const t = useT();
  const { deleteAccount } = useAuth();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const ok = confirmsDelete(typed, t("account.deleteWord"), user.name);

  function close() {
    setOpen(false);
    setTyped("");
    setError("");
  }

  async function onSubmit(e) {
    e.preventDefault();
    if (!ok || busy) return;
    setBusy(true);
    setError("");
    try {
      await deleteAccount();
      onDone();
    } catch (err) {
      setError(err.message);
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <button type="button" className="btn btn--danger" onClick={() => setOpen(true)}>
        {t("accountPanel.14")}
      </button>
    );
  }
  return (
    <form className="delform" onSubmit={onSubmit}>
      <label className="delform__label" htmlFor="delete-confirm">
        {t("accountPanel.15")} <b>{t("account.deleteWord")}</b> {t("accountPanel.16")} <b>{user.name}</b>.
      </label>
      <input
        id="delete-confirm"
        className="field delform__input"
        value={typed}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => setTyped(e.target.value)}
      />
      {error && (
        <p className="nameform__error" role="alert">
          {error}
        </p>
      )}
      <div className="savebar">
        <button type="button" className="btn btn--ghost" onClick={close} disabled={busy}>
          {t("common.cancel")}
        </button>
        <button type="submit" className="btn btn--danger" disabled={!ok || busy}>
          {t("accountPanel.18")}
        </button>
      </div>
    </form>
  );
}

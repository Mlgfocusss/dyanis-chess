import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { GoogleButton } from "../auth/AccountParts.jsx";
import { useT } from "../i18n/index.js";

// Shown in place of a settings section that needs an account: the same two ways in as the account page.
export default function SignInOffer() {
  const t = useT();
  const { status, google, guest } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (status === "loading") return null;
  if (status === "unavailable") {
    return <div className="banner banner--error">{t("signInOffer.1")}</div>;
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
    <div className="signin-offer">
      <p className="settings__sub">{t("signInOffer.2")}</p>
      {error && <div className="banner banner--error">{error}</div>}
      <div className="signin-offer__buttons">
        <GoogleButton enabled={google}>{t("auth.signInGoogle")}</GoogleButton>
        <button type="button" className="btn btn--block" disabled={busy} onClick={asGuest}>
          {t("auth.asGuest")}
        </button>
      </div>
    </div>
  );
}

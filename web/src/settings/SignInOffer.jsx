import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { GoogleButton } from "../auth/AccountParts.jsx";

// Shown in place of a settings section that needs an account: the same two ways in as the account page.
export default function SignInOffer() {
  const { status, google, guest } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (status === "loading") return null;
  if (status === "unavailable") {
    return <div className="banner banner--error">Аккаунты сейчас недоступны: сервер запущен без базы данных.</div>;
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
      <p className="settings__sub">Этот раздел доступен после входа. Войдите, чтобы партии и настройки были с вами на любом устройстве.</p>
      {error && <div className="banner banner--error">{error}</div>}
      <div className="signin-offer__buttons">
        <GoogleButton enabled={google}>Войти через Google</GoogleButton>
        <button type="button" className="btn btn--block" disabled={busy} onClick={asGuest}>
          Продолжить как гость
        </button>
      </div>
    </div>
  );
}

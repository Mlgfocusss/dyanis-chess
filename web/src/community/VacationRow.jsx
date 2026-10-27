import { useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { Row } from "../settings/controls.jsx";
import { setVacation } from "./api.js";
import { useCommunity } from "./CommunityContext.jsx";
import { daysText } from "./logic.js";
import { locale, useT } from "../i18n/index.js";

const CHOICES = [3, 7, 14];

// "Режим отпуска" in the settings: the clocks of the person's correspondence games stand still. The state is on
// the server (not in the settings document: it is a promise made to the opponents).
export default function VacationRow() {
  const t = useT();
  const { user } = useAuth();
  const { summary, refresh } = useCommunity();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const until = summary?.vacation || 0;
  const on = until > Date.now();

  async function go(days) {
    setBusy(true);
    setError("");
    try {
      await setVacation(days);
      await refresh();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Row
      title={t("vacation.1")}
      hint={t("vacation.2")}
    >
      {!user ? (
        <p className="srow__hint">{t("vacation.3")}</p>
      ) : on ? (
        <div className="vacation">
          <p className="vacation__line">
            {t("vacation.4")} <b>{new Date(until).toLocaleDateString(locale(), { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}</b>
          </p>
          <button type="button" className="btn" disabled={busy} onClick={() => go(0)}>
            {t("vacation.5")}
          </button>
        </div>
      ) : (
        <div className="vacation">
          {CHOICES.map((d) => (
            <button key={d} type="button" className="chip" disabled={busy} onClick={() => go(d)}>
              {t("vacation.6")} {daysText(d)}
            </button>
          ))}
        </div>
      )}
      {error && <p className="vacation__error">{error}</p>}
    </Row>
  );
}

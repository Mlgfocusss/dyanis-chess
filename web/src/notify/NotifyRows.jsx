import { useState } from "react";
import { Row, Switch } from "../settings/controls.jsx";
import { useT } from "../i18n/index.js";
import { playSound } from "../sound/sound.js";
import { askNotifyPermission, notifyPermission } from "./system.js";

// Two rows of "Настройки → Игровой процесс → Заочные партии": the chime for a move to answer and the
// system notice. The browser's permission is asked for here, when the switch is pressed, and nowhere else.
export default function NotifyRows({ draft, set }) {
  const t = useT();
  const [problem, setProblem] = useState(""); // "denied" | "unsupported" | ""

  async function toggleBrowser(on) {
    setProblem("");
    if (!on) return set({ corrNotifyBrowser: false });
    const answer = await askNotifyPermission();
    if (answer === "granted") return set({ corrNotifyBrowser: true });
    set({ corrNotifyBrowser: false });
    setProblem(answer === "unsupported" ? "unsupported" : "denied");
  }

  const granted = notifyPermission() === "granted";
  return (
    <>
      <Row
        title={t("notify.soundTitle")}
        hint={t("notify.soundHint")}
        control={<Switch checked={draft.corrNotifySound} onChange={(v) => set({ corrNotifySound: v })} label={t("notify.soundTitle")} />}
      >
        <button type="button" className="btn" onClick={() => playSound("notify", draft.soundTheme)}>
          {t("notify.soundTest")}
        </button>
      </Row>
      <Row
        title={t("notify.browserTitle")}
        hint={t("notify.browserHint")}
        control={<Switch checked={draft.corrNotifyBrowser && granted} onChange={toggleBrowser} label={t("notify.browserTitle")} />}
      >
        {problem && (
          <p className="srow__hint" role="status">
            {t(problem === "unsupported" ? "notify.browserUnsupported" : "notify.browserDenied")}
          </p>
        )}
        {!problem && draft.corrNotifyBrowser && granted && (
          <p className="srow__hint" role="status">
            {t("notify.browserOn")}
          </p>
        )}
      </Row>
    </>
  );
}

import { useT } from "../i18n/index.js";
import { useSettings } from "../settings/SettingsContext.jsx";
import { formatMoveTime } from "./moveTime.js";
import "./movetime.css";

// The units of the current language for formatMoveTime.
export function useMoveTimeFormat() {
  const t = useT();
  const units = { s: t("notify.unitS"), m: t("notify.unitM"), h: t("notify.unitH"), d: t("notify.unitD"), dec: t("notify.decimal") };
  return (ms) => formatMoveTime(ms, units);
}

// The small figure beside a move: how long it took. Nothing when the setting is off or the time is not known
// (a game played before the times were kept, a pasted PGN without clocks). `always` shows it whatever the
// setting says (the summary of the review asks for it by itself).
export default function MoveTime({ ms, always = false }) {
  const { settings } = useSettings();
  const fmt = useMoveTimeFormat();
  if ((!settings.showMoveTime && !always) || ms === null || ms === undefined) return null;
  const text = fmt(ms);
  if (!text) return null;
  return <span className="mtime">{text}</span>;
}

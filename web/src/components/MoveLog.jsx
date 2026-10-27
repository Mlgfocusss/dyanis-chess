import { useT } from "../i18n/index.js";
export default function MoveLog({ log }) {
  const t = useT();
  return (
    <div className="move-log">
      <h2>{t("play.gameTitle")}</h2>
      <pre className="move-log__text">{log || "—"}</pre>
    </div>
  );
}
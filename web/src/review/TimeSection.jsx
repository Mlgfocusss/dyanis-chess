import ClassBadge from "./ClassBadge.jsx";
import { moveLabel } from "./moments.js";
import { useT } from "../i18n/index.js";
import { useSettings } from "../settings/SettingsContext.jsx";
import { useMoveTimeFormat } from "../online/MoveTime.jsx";
import { averageTimes, hasMoveTimes, longestMoves } from "../online/moveTime.js";

// The "Время на ход" block of the summary: the average move of each side and the longest moves, which
// open on the board when pressed. Shown only when the setting "Показывать время хода" is on and the
// game had times (a clock in the PGN, or one of our own games).
export default function TimeSection({ moves, whiteName, blackName, ply, onOpen }) {
  const t = useT();
  const { settings } = useSettings();
  const fmt = useMoveTimeFormat();
  if (!settings.showMoveTime || !hasMoveTimes(moves)) return null;
  const avg = averageTimes(moves);
  const longest = longestMoves(moves, 3);
  return (
    <section className="sumtab__block">
      <h3>{t("notify.timeTitle")}</h3>
      <p className="timesec__sub">{t("notify.timeAverage")}</p>
      <div className="timesec__row">
        <span className="timesec__who">{whiteName}</span>
        <span className="timesec__num">{avg.white === null ? "—" : fmt(avg.white)}</span>
      </div>
      <div className="timesec__row">
        <span className="timesec__who">{blackName}</span>
        <span className="timesec__num">{avg.black === null ? "—" : fmt(avg.black)}</span>
      </div>
      {longest.length > 0 && (
        <>
          <p className="timesec__sub">{t("notify.timeLongest")}</p>
          <div className="moments">
            {longest.map((p) => {
              const m = moves[p - 1];
              return (
                <button key={p} type="button" className={`moment${p === ply ? " moment--on" : ""}`} onClick={() => onOpen(p)}>
                  <ClassBadge cls={m.class} />
                  <span className="moment__san">{moveLabel(m)}</span>
                  <small className="timesec__time">{fmt(m.timeMs)}</small>
                </button>
              );
            })}
          </div>
        </>
      )}
    </section>
  );
}

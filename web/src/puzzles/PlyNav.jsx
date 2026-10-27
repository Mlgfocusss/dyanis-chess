import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";

// The moves of the puzzle so far, with the buttons to wind them back and forth (the keys ← → Home End do
// the same) and, once the puzzle is over, the way to the analysis board from the position on the board.
// trail: [{ fen, last, san }] (the first one is the start); at: the shown one; analysisHref is null until then.
export default function PlyNav({ trail, at, onGo, analysisHref }) {
  const t = useT();
  const live = trail.length - 1;
  return (
    <div className="pz__ply">
      <div className="pz__plyhead">
        <span className="pz__label">{t("pznav.title")}</span>
        <div className="pz__plybtns">
          <button type="button" className="pz__plybtn" onClick={() => onGo(0)} disabled={at <= 0} title={t("pznav.first")} aria-label={t("pznav.first")}>
            <Icon name="first" size={16} />
          </button>
          <button type="button" className="pz__plybtn" onClick={() => onGo(at - 1)} disabled={at <= 0} title={t("pznav.prev")} aria-label={t("pznav.prev")}>
            <Icon name="prev" size={16} />
          </button>
          <button type="button" className="pz__plybtn" onClick={() => onGo(at + 1)} disabled={at >= live} title={t("pznav.next")} aria-label={t("pznav.next")}>
            <Icon name="next" size={16} />
          </button>
          <button type="button" className="pz__plybtn" onClick={() => onGo(live)} disabled={at >= live} title={t("pznav.last")} aria-label={t("pznav.last")}>
            <Icon name="last" size={16} />
          </button>
        </div>
      </div>
      <div className="pz__moves">
        {trail.map((e, i) =>
          i === 0 ? null : (
            <button key={i} type="button" className={`pz__mv${i === at ? " pz__mv--on" : ""}`} onClick={() => onGo(i)}>
              {e.san}
            </button>
          )
        )}
        {trail.length < 2 && <span className="pz__hintline">{t("pznav.empty")}</span>}
      </div>
      {at < live && <p className="pz__hintline">{t("pznav.viewing")}</p>}
      <a
        className="btn btn--ghost pz__analysis"
        href={analysisHref || undefined}
        aria-disabled={!analysisHref}
        title={analysisHref ? t("pznav.analysisTitle") : t("pznav.analysisLocked")}
        onClick={(e) => {
          if (!analysisHref) e.preventDefault();
        }}
      >
        <Icon name="board" size={16} /> {t("pznav.analysis")}
      </a>
    </div>
  );
}

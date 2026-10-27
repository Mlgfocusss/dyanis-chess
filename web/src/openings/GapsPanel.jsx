import { useState } from "react";
import { useT } from "../i18n/index.js";
import { formatGames } from "./suggest.js";
import { MIN_PCT, coveragePercent, formatWeight, gapCounts, keyOf, moveLabel, sourceOf } from "./gaps.js";
import { lineText, pathTo } from "./tree.js";

const SHOWN = 4; // holes listed before "show all"
const TAIL = 8; // plies of the line shown in a row

// The line that leads to a hole, shortened from the front.
function lineOf(tree, g) {
  const node = g.kind === "answer" ? tree.byId.get(g.at)?.parent ?? 0 : g.at;
  const path = pathTo(tree, node);
  const tail = path.slice(-TAIL);
  return (path.length > tail.length ? "… " : "") + lineText(tail);
}

// What the move of a hole is called: "3... c5".
export function holeMove(g) {
  return g.kind === "answer" ? moveLabel(g.ply - 1, g.san) : moveLabel(g.ply, g.san);
}

// The panel of holes of the builder: how much of the likely games the repository answers, the
// popular replies of the opponent it has no answer to (the heaviest first), "next hole" and the
// mode that leads from one hole to the next as answers are added.
// items: the holes to show (gaps.js visibleGaps); scan: the last answer of the server (or null);
// state: "loading" (no answer yet), "ready" or "error"; focusKey: the hole being worked on.
export default function GapsPanel({ tree, cur, items, scan, state, refreshing, error, focusKey, fill, fillStatus, hasMoves, onFill, onGo, onNext, onAdd, onRecheck }) {
  const t = useT();
  const [all, setAll] = useState(false);
  if (!hasMoves) {
    return (
      <section className="card gaps">
        <div className="gaps__head">
          <h2>{t("openings2.gaps.title")}</h2>
        </div>
        <p className="room__meta">{t("openings2.gaps.rootEmpty")}</p>
      </section>
    );
  }
  const counts = gapCounts(items);
  const focused = items.find((g) => keyOf(g) === focusKey) || null;
  const shown = all ? items : items.slice(0, SHOWN);
  const src = sourceOf(scan);
  const pct = scan ? coveragePercent(scan.coverage, items.length) : 0;
  const waiting = scan?.limited ? scan.retryAfter : 0;
  const note = error
    ? t("openings2.gaps.error", { msg: error })
    : !scan
      ? ""
      : waiting
        ? t("openings2.gaps.wait", { s: waiting })
        : !scan.available
          ? t("openings2.gaps.noToken")
          : src === "mixed"
            ? t("openings2.gaps.mixed")
            : scan.partial
              ? t("openings2.gaps.partial", { n: scan.counts?.pending || 0 })
              : "";

  return (
    <section className="card gaps" aria-busy={refreshing}>
      <div className="gaps__head">
        <h2>{t("openings2.gaps.title")}</h2>
        {state === "ready" && <span className={`gaps__count${items.length ? "" : " gaps__count--ok"}`}>{t("openings2.gaps.count", { n: items.length })}</span>}
        <button type="button" className="gaps__recheck" onClick={onRecheck} disabled={refreshing} title={t("openings2.gaps.recheckTitle")}>
          {refreshing ? t("openings2.gaps.refreshing") : t("openings2.gaps.recheck")}
        </button>
      </div>

      <div className="gaps__cov">
        <div className="gaps__covtext" title={t("openings2.gaps.coverageNote", { pct: MIN_PCT })}>
          {!scan ? (
            <span className="gaps__muted">{state === "error" ? "" : t("openings2.gaps.checking")}</span>
          ) : (
            <>
              <b>{scan.partial ? t("openings2.gaps.coverageAtLeast", { p: pct }) : t("openings2.gaps.coverage", { p: pct })}</b>
              {scan.stale && <span className="gaps__muted"> · {t("openings2.gaps.stale")}</span>}
            </>
          )}
        </div>
        <div className="gaps__bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={t("openings2.gaps.title")}>
          <div className={`gaps__fill${scan?.stale || refreshing ? " gaps__fill--stale" : ""}`} style={{ width: `${scan ? pct : 0}%` }} />
        </div>
        {scan && src === "theory" && !waiting && scan.available && <p className="gaps__muted">{t("openings2.gaps.theory")}</p>}
      </div>

      {note && <p className={`gaps__note${error ? " gaps__note--err" : ""}`}>{note}</p>}

      <div className="gaps__actions">
        <button type="button" className="btn btn--primary" onClick={onNext} disabled={!items.length} title={t("openings2.gaps.nextTitle")}>
          {t("openings2.gaps.next")} <kbd className="gaps__key">G</kbd>
        </button>
        <label className="gaps__switch" title={t("openings2.gaps.fillHint")}>
          <input type="checkbox" checked={fill} onChange={(e) => onFill(e.target.checked)} />
          <span>{t("openings2.gaps.fill")}</span>
        </label>
      </div>
      {fill && fillStatus && (
        <p className="gaps__status" role="status">
          {fillStatus === "searching" ? t("openings2.gaps.fillSearching") : fillStatus === "done" ? t("openings2.gaps.fillDone") : ""}
        </p>
      )}

      {focused && (
        <div className="gaps__focus" role="status">
          <p>
            {focused.kind === "answer"
              ? t("openings2.gaps.focusAnswer", { move: holeMove(focused) })
              : focused.source === "theory"
                ? t("openings2.gaps.focusTheory", { move: holeMove(focused) })
                : t("openings2.gaps.focusMissing", { move: holeMove(focused), pct: Math.round(focused.pct) })}
          </p>
          {focused.kind === "missing" && cur === focused.at && (
            <button type="button" className="btn btn--primary" onClick={() => onAdd(focused)}>
              {t("openings2.gaps.addOpp", { san: focused.san })}
            </button>
          )}
          {cur !== focused.at && (
            <button type="button" className="btn" onClick={() => onGo(focused)}>
              {t("openings2.gaps.goHere")}
            </button>
          )}
        </div>
      )}

      {state === "ready" && items.length === 0 && <p className="gaps__good">{t("openings2.gaps.none")}</p>}
      {items.length > 0 && (
        <ul className="gaps__list">
          {shown.map((g) => {
            const key = keyOf(g);
            const w = formatWeight(g.weight);
            const title =
              g.kind === "answer"
                ? t("openings2.gaps.rowAnswerTitle", { san: g.san })
                : g.source === "theory"
                  ? t("openings2.gaps.rowTheoryTitle", { san: g.san, w })
                  : t("openings2.gaps.rowTitle", { san: g.san, pct: Math.round(g.pct), games: formatGames(g.games), w });
            return (
              <li key={key}>
                <button type="button" className={`gaprow${key === focusKey ? " gaprow--on" : ""}`} onClick={() => onGo(g)} title={title}>
                  <span className="gaprow__mv">{holeMove(g)}</span>
                  <span className="gaprow__mid">
                    <span className="gaprow__line">{lineOf(tree, g) || t("openings2.gaps.start")}</span>
                    <span className={`gaprow__kind gaprow__kind--${g.kind}`}>{g.kind === "answer" ? t("openings2.gaps.rowAnswer") : t("openings2.gaps.rowMissing")}</span>
                  </span>
                  <span className="gaprow__w">{w}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {items.length > SHOWN && (
        <button type="button" className="btn btn--ghost gaps__more" onClick={() => setAll((v) => !v)}>
          {all ? t("openings2.gaps.showLess") : t("openings2.gaps.showAll", { n: items.length })}
        </button>
      )}
      {counts.answer > 0 && counts.missing > 0 && <p className="gaps__muted">{t("openings2.gaps.kinds", { missing: counts.missing, answer: counts.answer })}</p>}
    </section>
  );
}

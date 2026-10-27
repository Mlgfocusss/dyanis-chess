import { useEffect, useMemo, useRef, useState } from "react";
import { fetchContinuations, fetchExplorer } from "./api.js";
import { fetchLine } from "../review/api.js";
import { formatEval } from "../review/classes.js";
import { statusOf } from "./tree.js";
import { openingName } from "./names.js";
import { RATING_PRESETS, buildSuggestions, formatGames, formatShare, loadPreset, makeQueue, ratingsOf, resultShares, storePreset } from "./suggest.js";
import { formatWeight, isPopularReply } from "./gaps.js";
import { useT } from "../i18n/index.js";

const CAND_DEPTH = 10; // depth of the verdict on each candidate move
const POS_DEPTH = 12; // depth of the verdict on the position itself
const MAX_CANDIDATES = 10; // moves that get a verdict (the most played ones)
const SHOWN = 8; // rows shown before "show more"
const DEBOUNCE_MS = 350; // stepping through positions quickly asks for nothing

const STATUS_TEXT = { opponent: "openings.status.opponent", new: "openings.status.new", due: "openings.status.due", learning: "openings.status.learning", learned: "openings.status.learned" };

// The moves of the position as the builder offers them: those of the repository, those
// that were played in real games (the Lichess statistics: how many games, how they
// ended), those opening theory knows (the ECO reference), and the engine's own
// choice, each with the engine's verdict on the position after it. A click on a row
// plays it (adds it to the repository, or goes along if it is there already).
// side is who moves, "white" or "black". When the opponent moves, a popular reply the repository lacks
// is marked as a hole; gaps (optional) are the holes the server's search found at this position,
// a Map from the move (UCI) to the hole, which adds their weight. children are the moves the repository has here, with those
// of other branches to the same position (a move whose parent is not `here` is marked as a transposition). preset and onPreset (optional)
// let the page share the rating range with the search for holes.
export default function Suggestions({ fen, side, turn, children, here, onPick, onHover, gaps, preset: presetProp, onPreset }) {
  const t = useT();
  const [theory, setTheory] = useState(null); // moves for this fen, null while loading
  const [stats, setStats] = useState({ state: "loading", data: null }); // state: loading | ready | off | wait | error
  const [localPreset, setLocalPreset] = useState(loadPreset);
  const preset = presetProp || localPreset;
  const [evAfter, setEvAfter] = useState({}); // `${fen}|${uci}` -> verdict
  const [evPos, setEvPos] = useState({}); // fen -> verdict
  const [all, setAll] = useState(false);
  const [error, setError] = useState("");
  const run = useRef(makeQueue(2)).current;
  const theoryCache = useRef(new Map());
  const statsCache = useRef(new Map());
  const gen = useRef(0);
  const ratings = ratingsOf(preset);

  // theory for the position
  useEffect(() => {
    setAll(false);
    setError("");
    const cached = theoryCache.current.get(fen);
    if (cached) {
      setTheory(cached);
      return undefined;
    }
    setTheory(null);
    let alive = true;
    fetchContinuations(fen)
      .then((r) => {
        theoryCache.current.set(fen, r.moves);
        if (alive) setTheory(r.moves);
      })
      .catch((e) => alive && (setTheory([]), setError(e.message)));
    return () => {
      alive = false;
    };
  }, [fen]);

  // the statistics of real games for the position and the chosen rating range
  useEffect(() => {
    const key = `${fen}|${ratings}`;
    const cached = statsCache.current.get(key);
    if (cached) {
      setStats(cached);
      return undefined;
    }
    setStats((s) => ({ state: "loading", data: s.state === "ready" ? null : s.data }));
    let alive = true;
    fetchExplorer(fen, ratings)
      .then((data) => {
        const next = data.available ? { state: "ready", data } : { state: "off", data: null };
        statsCache.current.set(key, next);
        if (alive) setStats(next);
      })
      .catch((e) => alive && setStats({ state: e.status === 429 ? "wait" : "error", data: null }));
    return () => {
      alive = false;
    };
  }, [fen, ratings]);

  function choosePreset(id) {
    setLocalPreset(id);
    storePreset(id);
    if (onPreset) onPreset(id);
  }

  const played = stats.state === "ready" ? stats.data : null;
  const candidates = useMemo(() => {
    const seen = new Set();
    const list = [];
    for (const m of children) if (!seen.has(m.uci) && seen.add(m.uci)) list.push(m.uci);
    const known = played && played.moves.length ? played.moves : theory || [];
    for (const t of known.slice(0, MAX_CANDIDATES)) if (!seen.has(t.uci) && seen.add(t.uci)) list.push(t.uci);
    return list;
  }, [children, theory, played]);

  // the verdict on the position, then on each candidate (and on the engine's own choice)
  const bestUci = evPos[fen]?.bestUci;
  useEffect(() => {
    if (theory === null) return;
    const mine = ++gen.current;
    const alive = () => gen.current === mine;
    const ask = (key, store, set, req) => {
      if (store[key]) return;
      run(async () => {
        if (!alive()) return;
        await new Promise((r) => setTimeout(r, DEBOUNCE_MS));
        if (!alive()) return;
        try {
          const r = await fetchLine(req);
          if (r.eval) set((c) => ({ ...c, [key]: r.eval }));
        } catch (e) {
          if (alive()) setError(e.message);
        }
      });
    };
    ask(fen, evPos, setEvPos, { fen, eval: true, depth: POS_DEPTH });
    const list = bestUci && !candidates.includes(bestUci) ? [...candidates, bestUci] : candidates;
    for (const uci of list) ask(`${fen}|${uci}`, evAfter, setEvAfter, { fen, moves: [uci], eval: true, depth: CAND_DEPTH });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fen, theory, candidates, bestUci]);

  const before = evPos[fen] || null;
  const rows = useMemo(() => {
    const after = new Map();
    for (const uci of [...candidates, ...(bestUci ? [bestUci] : [])]) {
      const ev = evAfter[`${fen}|${uci}`];
      if (ev) after.set(uci, ev);
    }
    return buildSuggestions({
      children,
      theory: theory || [],
      stats: played,
      best: before?.bestUci ? { uci: before.bestUci, san: before.bestSan } : null,
      after,
      before,
      side,
    });
  }, [children, theory, played, before, evAfter, candidates, bestUci, fen, side]);

  const shown = all ? rows : rows.slice(0, SHOWN);
  const hasStats = !!played && played.moves.length > 0;
  return (
    <div className="sugg">
      <div className="sugg__eval" title={t("suggestions.6")}>
        <div className="sugg__bar">
          <div className="sugg__barfill" style={{ width: `${before ? Math.max(0, Math.min(100, before.winPct)) : 50}%` }} />
        </div>
        <span className="sugg__evtext">
          {before ? (
            <>
              <b>{formatEval(before)}</b>
              {before.bestSan && !before.terminal && <span> {t("suggestions.7")} {before.bestSan}</span>}
              {before.terminal && <span> · {before.terminal === "checkmate" ? t("reviewView.42") : before.terminal}</span>}
            </>
          ) : (
            t("suggestions.9")
          )}
        </span>
      </div>

      <div className="sugg__head">
        <p className="sugg__title">
          {turn === "mine" ? t("suggestions.10") : t("suggestions.11")}
          {theory === null && <span className="sugg__wait"> {t("suggestions.12")}</span>}
        </p>
        {stats.state !== "off" && (
          <label className="sugg__rating" title={t("suggestions.13")}>
            <span>{t("rating.rating")}</span>
            <select className="field" value={preset} onChange={(e) => choosePreset(e.target.value)} aria-label={t("suggestions.15")}>
              {RATING_PRESETS.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label.startsWith("suggest.") ? t(p.label) : p.label}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      {played && played.total > 0 && (
        <p className="room__meta sugg__total">
          {t("suggestions.16")} {formatGames(played.total)} {t("suggestions.17")} {resultShares(played).white}{t("suggestions.18")} {resultShares(played).draws}{t("suggestions.19")} {resultShares(played).black}%.
        </p>
      )}

      {rows.length === 0 && theory !== null && stats.state !== "loading" && (
        <p className="room__meta">{t("suggestions.20")}</p>
      )}

      <div className="sugg__rows" role="list" onMouseLeave={() => onHover && onHover(null)}>
        {shown.map((r) => {
          const res = r.stat ? resultShares(r) : null;
          const hole = turn === "opponent" && !r.repo && (gaps?.has(r.uci) || isPopularReply(r, hasStats));
          const holeWeight = hole && gaps?.get(r.uci) ? formatWeight(gaps.get(r.uci).weight) : "";
          return (
            <button
              key={r.uci}
              type="button"
              role="listitem"
              className={`sug${r.repo ? " sug--repo" : ""}${r.weak ? " sug--weak" : ""}${r.rare ? " sug--rare" : ""}${hole ? " sug--gap" : ""}`}
              onClick={() => onPick(r.uci)}
              onMouseEnter={() => onHover && onHover(r.uci)}
              onFocus={() => onHover && onHover(r.uci)}
              title={r.opening ? `${r.opening.eco} ${r.opening.name}` : undefined}
            >
              <span className="sug__san">{r.san}</span>
              <span className="sug__mid">
                {r.stat ? (
                  <span className="sug__games" title={t("suggestions.21", { games: r.games, p: r.averageRating || "?" })}>
                    <b>{formatShare(r.share)}</b> · {formatGames(r.games)}
                  </span>
                ) : (
                  r.theory && (
                    <span className="sug__pop" title={t("suggestions.22", { lines: r.lines })}>
                      <span className="sug__popfill" style={{ width: `${Math.max(4, Math.round(r.share * 100))}%` }} />
                    </span>
                  )
                )}
                {res && (
                  <span className="sug__res" title={t("suggestions.23", { white: res.white, draws: res.draws, black: res.black })}>
                    <span className="sug__w" style={{ width: `${res.white}%` }} />
                    <span className="sug__d" style={{ width: `${res.draws}%` }} />
                    <span className="sug__b" style={{ width: `${res.black}%` }} />
                  </span>
                )}
                <span className="sug__tags">
                  {r.repo && (
                    <span className="sug__tag sug__tag--repo" title={r.repo.mine ? t(STATUS_TEXT[statusOf(r.repo)]) : t("suggestions.24")}>
                      {t("suggestions.25")}
                    </span>
                  )}
                  {r.repo && here !== undefined && r.repo.parent !== here && (
                    <span className="sug__tag sug__tag--trans" title={t("openings2.trans.tagTitle")}>
                      {t("openings2.trans.tag")}
                    </span>
                  )}
                  {hole && (
                    <span className="sug__tag sug__tag--gap" title={holeWeight ? t("openings2.gaps.tagWeight", { w: holeWeight }) : t("openings2.gaps.tagTitle")}>
                      {t("openings2.gaps.tag")}
                    </span>
                  )}
                  {r.engine && (
                    <span className="sug__tag sug__tag--engine" title={t("suggestions.26")}>
                      {t("suggestions.27")}
                    </span>
                  )}
                  {r.rare && <span className="sug__tag sug__tag--muted" title={t("suggestions.28")}>{t("suggestions.29")}</span>}
                  <span className="sug__open">{r.opening ? openingName(r.opening) : r.theory || r.stat ? "" : t("suggestions.30")}</span>
                </span>
              </span>
              <span className="sug__ev">
                {r.ev ? (
                  <>
                    <b>{formatEval(r.ev)}</b>
                    {r.loss !== null && r.loss >= 2 && <span className={`sug__loss${r.weak ? " sug__loss--bad" : r.loss >= 5 ? " sug__loss--warn" : ""}`}>−{r.loss.toFixed(0)}%</span>}
                  </>
                ) : (
                  <span className="sug__pending">…</span>
                )}
              </span>
              <span className="sug__act" title={r.repo ? t("suggestions.goto") : t("suggestions.32")}>
                {r.repo ? "→" : "+"}
              </span>
            </button>
          );
        })}
      </div>
      {rows.length > SHOWN && (
        <button type="button" className="btn btn--ghost sugg__more" onClick={() => setAll((v) => !v)}>
          {all ? t("suggestions.33") : t("suggestions.34", { SHOWN: rows.length - SHOWN })}
        </button>
      )}
      {error && <p className="room__meta sugg__error">{error}</p>}
      <p className="sugg__note">
        {hasStats
          ? t("suggestions.35")
          : stats.state === "off"
            ? t("suggestions.36")
            : stats.state === "wait"
              ? t("suggestions.37")
              : stats.state === "error"
                ? t("suggestions.38")
                : ""}
        {t("suggestions.39")}
      </p>
    </div>
  );
}

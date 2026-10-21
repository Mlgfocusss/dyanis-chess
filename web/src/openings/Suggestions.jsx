import { useEffect, useMemo, useRef, useState } from "react";
import { fetchContinuations, fetchExplorer } from "./api.js";
import { fetchLine } from "../review/api.js";
import { formatEval } from "../review/classes.js";
import { statusOf } from "./tree.js";
import { openingName } from "./names.js";
import { DEFAULT_PRESET, RATING_PRESETS, buildSuggestions, formatGames, formatShare, makeQueue, resultShares } from "./suggest.js";

const CAND_DEPTH = 10; // depth of the verdict on each candidate move
const POS_DEPTH = 12; // depth of the verdict on the position itself
const MAX_CANDIDATES = 10; // moves that get a verdict (the most played ones)
const SHOWN = 8; // rows shown before "show more"
const DEBOUNCE_MS = 350; // stepping through positions quickly asks for nothing
const PRESET_KEY = "dyanis.explorerRating";

const STATUS_TEXT = { opponent: "ход соперника", new: "новый", due: "пора повторить", learning: "учится", learned: "выучен" };

function savedPreset() {
  try {
    const id = localStorage.getItem(PRESET_KEY);
    return RATING_PRESETS.some((p) => p.id === id) ? id : DEFAULT_PRESET;
  } catch {
    return DEFAULT_PRESET;
  }
}

// The moves of the position as the builder offers them: those of the repository, those
// that were played in real games (the Lichess statistics: how many games, how they
// ended), those opening theory knows (the ECO reference), and the engine's own
// choice, each with the engine's verdict on the position after it. A click on a row
// plays it (adds it to the repository, or goes along if it is there already).
// side is who moves, "white" or "black".
export default function Suggestions({ fen, side, turn, children, onPick, onHover }) {
  const [theory, setTheory] = useState(null); // moves for this fen, null while loading
  const [stats, setStats] = useState({ state: "loading", data: null }); // state: loading | ready | off | wait | error
  const [preset, setPreset] = useState(savedPreset);
  const [evAfter, setEvAfter] = useState({}); // `${fen}|${uci}` -> verdict
  const [evPos, setEvPos] = useState({}); // fen -> verdict
  const [all, setAll] = useState(false);
  const [error, setError] = useState("");
  const run = useRef(makeQueue(2)).current;
  const theoryCache = useRef(new Map());
  const statsCache = useRef(new Map());
  const gen = useRef(0);
  const ratings = (RATING_PRESETS.find((p) => p.id === preset) || RATING_PRESETS[1]).ratings;

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
    setPreset(id);
    try {
      localStorage.setItem(PRESET_KEY, id);
    } catch {
      // a private window: the choice is just not remembered
    }
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
      <div className="sugg__eval" title="Вероятность победы белых по оценке движка">
        <div className="sugg__bar">
          <div className="sugg__barfill" style={{ width: `${before ? Math.max(0, Math.min(100, before.winPct)) : 50}%` }} />
        </div>
        <span className="sugg__evtext">
          {before ? (
            <>
              <b>{formatEval(before)}</b>
              {before.bestSan && !before.terminal && <span> · по движку лучше {before.bestSan}</span>}
              {before.terminal && <span> · {before.terminal === "checkmate" ? "мат" : before.terminal}</span>}
            </>
          ) : (
            "оцениваю позицию…"
          )}
        </span>
      </div>

      <div className="sugg__head">
        <p className="sugg__title">
          {turn === "mine" ? "Ваши ходы здесь" : "Что может сыграть соперник"}
          {theory === null && <span className="sugg__wait"> · загружаю теорию…</span>}
        </p>
        {stats.state !== "off" && (
          <label className="sugg__rating" title="Рейтинг игроков, чьи партии учитываются в статистике Lichess">
            <span>Рейтинг</span>
            <select className="field" value={preset} onChange={(e) => choosePreset(e.target.value)} aria-label="Рейтинг игроков в статистике">
              {RATING_PRESETS.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      {played && played.total > 0 && (
        <p className="room__meta sugg__total">
          В {formatGames(played.total)} партий Lichess с этой позиции. Белые выиграли в {resultShares(played).white}%, ничьих {resultShares(played).draws}%, чёрные выиграли в {resultShares(played).black}%.
        </p>
      )}

      {rows.length === 0 && theory !== null && stats.state !== "loading" && (
        <p className="room__meta">Теория здесь закончилась. Сделайте на доске ход, который хотите добавить: движок оценит позицию.</p>
      )}

      <div className="sugg__rows" role="list" onMouseLeave={() => onHover && onHover(null)}>
        {shown.map((r) => {
          const res = r.stat ? resultShares(r) : null;
          return (
            <button
              key={r.uci}
              type="button"
              role="listitem"
              className={`sug${r.repo ? " sug--repo" : ""}${r.weak ? " sug--weak" : ""}${r.rare ? " sug--rare" : ""}`}
              onClick={() => onPick(r.uci)}
              onMouseEnter={() => onHover && onHover(r.uci)}
              onFocus={() => onHover && onHover(r.uci)}
              title={r.opening ? `${r.opening.eco} ${r.opening.name}` : undefined}
            >
              <span className="sug__san">{r.san}</span>
              <span className="sug__mid">
                {r.stat ? (
                  <span className="sug__games" title={`Сыграно ${r.games} партий; в среднем рейтинг ${r.averageRating || "?"}`}>
                    <b>{formatShare(r.share)}</b> · {formatGames(r.games)}
                  </span>
                ) : (
                  r.theory && (
                    <span className="sug__pop" title={`${r.lines} линий теории проходят через этот ход`}>
                      <span className="sug__popfill" style={{ width: `${Math.max(4, Math.round(r.share * 100))}%` }} />
                    </span>
                  )
                )}
                {res && (
                  <span className="sug__res" title={`Белые выиграли в ${res.white}%, ничьих ${res.draws}%, чёрные выиграли в ${res.black}%`}>
                    <span className="sug__w" style={{ width: `${res.white}%` }} />
                    <span className="sug__d" style={{ width: `${res.draws}%` }} />
                    <span className="sug__b" style={{ width: `${res.black}%` }} />
                  </span>
                )}
                <span className="sug__tags">
                  {r.repo && (
                    <span className="sug__tag sug__tag--repo" title={r.repo.mine ? STATUS_TEXT[statusOf(r.repo)] : "ответ соперника, который вы ждёте"}>
                      в репозитории
                    </span>
                  )}
                  {r.engine && (
                    <span className="sug__tag sug__tag--engine" title="Лучший ход по оценке движка">
                      лучший
                    </span>
                  )}
                  {r.rare && <span className="sug__tag sug__tag--muted" title="Этот ход играют редко">редкий</span>}
                  <span className="sug__open">{r.opening ? openingName(r.opening) : r.theory || r.stat ? "" : "вне теории"}</span>
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
              <span className="sug__act" title={r.repo ? "Перейти к ходу" : "Сыграть и добавить в репозиторий"}>
                {r.repo ? "→" : "+"}
              </span>
            </button>
          );
        })}
      </div>
      {rows.length > SHOWN && (
        <button type="button" className="btn btn--ghost sugg__more" onClick={() => setAll((v) => !v)}>
          {all ? "Свернуть" : `Показать ещё ${rows.length - SHOWN}`}
        </button>
      )}
      {error && <p className="room__meta sugg__error">{error}</p>}
      <p className="sugg__note">
        {hasStats
          ? "Процент и число: сколько партий Lichess выбрали этот ход, цветная полоса: чем они кончились (светлое: белые, серое: ничьи, тёмное: чёрные). "
          : stats.state === "off"
            ? "Статистика партий Lichess не подключена (нужен токен, см. README): ходы упорядочены по теории справочника ECO, полоса показывает, сколько его линий проходит через ход. "
            : stats.state === "wait"
              ? "Lichess просит подождать, статистика скоро вернётся: пока ходы по теории справочника ECO. "
              : stats.state === "error"
                ? "Статистику Lichess получить не удалось: пока ходы по теории справочника ECO. "
                : ""}
        Оценка: движок, белые в плюсе при «+», у слабых ходов указана потеря шансов. Нажмите на ход, чтобы сыграть его и добавить в репозиторий.
      </p>
    </div>
  );
}

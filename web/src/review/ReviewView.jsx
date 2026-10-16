import { useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Captured from "../components/Captured.jsx";
import { useMoveSound, useSettings } from "../settings/SettingsContext.jsx";
import { buildPgn } from "../play/game.js";
import EvalChart from "./EvalChart.jsx";
import MoveList from "./MoveList.jsx";
import ClassBadge from "./ClassBadge.jsx";
import Icon from "../ui/Icon.jsx";
import { CLASSES, SUMMARY_ORDER, formatEval, uciSquares } from "./classes.js";
import { fetchLine } from "./api.js";
import { judgeBranchMove, lineText, numberedSans, sideOf } from "./branch.js";
import { copyText } from "./clipboard.js";
import "./review.css";

const WHITE_NAME = "Белые";
const BLACK_NAME = "Чёрные";

// inline: shown in place of a game screen (the review of a game just played)
// instead of on its own page; onBack then returns to the game. orientation is
// the side the board starts from.
export default function ReviewView({ job, inline = false, orientation: startSide = "white", onBack }) {
  const { report, game } = job;
  const { evals, moves } = report;
  const last = moves.length;

  const depth = report.options?.depth || 0; // the depth of the review; 0 lets the server choose

  const [ply, setPly] = useState(inline ? last : 0); // a review of the game just played starts where the game ended
  const [orientation, setOrientation] = useState(startSide);
  const [showBest, setShowBest] = useState(true);
  const { settings } = useSettings();
  const playMoveSound = useMoveSound();

  // The person's own branch: moves played on the board from a position of the
  // game (or taken from the engine's line). base is the ply it starts from,
  // steps its moves, at how many of them are on the board.
  const [branch, setBranch] = useState(null);
  const [evCache, setEvCache] = useState(() => Object.fromEntries(evals.map((e) => [e.fen, e])));
  const [legalCache, setLegalCache] = useState({});
  const [notice, setNotice] = useState("");
  const [boardKey, setBoardKey] = useState(0);
  const [copied, setCopied] = useState("");
  const asking = useRef(new Set()); // requests in flight, by kind and position
  const seq = useRef(0);

  const go = (p) => {
    setBranch(null);
    setNotice("");
    setPly(Math.max(0, Math.min(last, p)));
  };

  // The position on the board, and the one before it inside a branch.
  const baseFen = evals[branch ? branch.base : ply].fen;
  const curFen = !branch ? baseFen : branch.at === 0 ? baseFen : branch.steps[branch.at - 1].fen;
  const parentFen = branch && branch.at > 0 ? (branch.at === 1 ? baseFen : branch.steps[branch.at - 2].fen) : null;
  const lastStep = branch && branch.at > 0 ? branch.steps[branch.at - 1] : null;

  function stepBack() {
    if (!branch) return go(ply - 1);
    if (branch.at > 0) return setBranch({ ...branch, at: branch.at - 1 });
    go(branch.base - 1);
  }
  function stepForward() {
    if (!branch) return go(ply + 1);
    if (branch.at < branch.steps.length) {
      playMoveSound(branch.steps[branch.at].san);
      setBranch({ ...branch, at: branch.at + 1 });
    }
  }
  const toEnd = () => (branch ? setBranch({ ...branch, at: branch.steps.length }) : go(last));
  const atStart = !branch && ply === 0;
  const atEnd = branch ? branch.at === branch.steps.length : ply === last;

  useEffect(() => {
    function onKey(e) {
      const tag = e.target.tagName;
      if (tag === "TEXTAREA" || tag === "INPUT" || e.altKey || e.ctrlKey || e.metaKey) return;
      if (e.key === "ArrowLeft") stepBack();
      else if (e.key === "ArrowRight") stepForward();
      else if (e.key === "Home") go(0);
      else if (e.key === "End") toEnd();
      else if (e.key === "Escape" && branch) go(branch.base);
      else return;
      e.preventDefault();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const played = !branch && ply > 0 ? moves[ply - 1] : null;

  // Stepping forward one move makes that move's sound; jumps are silent.
  const prevPly = useRef(ply);
  useEffect(() => {
    if (!branch && ply === prevPly.current + 1) playMoveSound(moves[ply - 1].san);
    prevPly.current = ply;
  }, [ply]); // eslint-disable-line react-hooks/exhaustive-deps

  // The legal moves of the position on the board come from the server.
  useEffect(() => {
    if (legalCache[curFen] || asking.current.has("legal" + curFen)) return;
    asking.current.add("legal" + curFen);
    fetchLine({ fen: curFen })
      .then((r) => setLegalCache((c) => ({ ...c, [curFen]: r.legal })))
      .catch(() => {})
      .finally(() => asking.current.delete("legal" + curFen));
  }, [curFen, legalCache]);

  // Inside a branch the engine judges the position on the board and the one
  // before it (to say how good the last move was).
  useEffect(() => {
    if (!branch) return;
    for (const fen of [curFen, parentFen]) {
      if (!fen || evCache[fen] || asking.current.has("eval" + fen)) continue;
      asking.current.add("eval" + fen);
      fetchLine({ fen, eval: true, depth })
        .then((r) => r.eval && setEvCache((c) => ({ ...c, [fen]: r.eval })))
        .catch((e) => setNotice(e.message))
        .finally(() => asking.current.delete("eval" + fen));
    }
  }, [branch, curFen, parentFen, evCache, depth]);

  // What the evaluation shows: the position's own, or the last known one while it is being worked out.
  const [shownEv, setShownEv] = useState(evals[ply]);
  const curEv = evCache[curFen];
  useEffect(() => {
    if (curEv) setShownEv(curEv);
  }, [curEv]);
  const ev = curEv || shownEv;
  const thinking = !curEv;

  // The opening is the latest name reached up to the current ply.
  const openingPly = branch ? branch.base : ply;
  const opening = useMemo(() => {
    for (let i = openingPly - 1; i >= 0; i--) if (moves[i].opening) return moves[i].opening;
    return null;
  }, [moves, openingPly]);

  // The move just made (on the main line or in the branch) and how it was judged.
  const lastUci = lastStep ? lastStep.uci : branch ? moves[branch.base - 1]?.uci : played?.uci;
  const branchJudge = lastStep ? judgeBranchMove(lastStep, evCache[parentFen], evCache[curFen], sideOf(parentFen)) : null;
  const lastClass = lastStep ? branchJudge?.cls : played?.class;
  const lastMoveColor = lastClass === "blunder" || lastClass === "mistake" ? "rgba(251, 146, 60, 0.6)" : undefined;
  const lastSquares = uciSquares(lastUci);

  const arrows = useMemo(() => {
    const b = uciSquares(curEv?.bestUci);
    return showBest && b ? [[b.from, b.to, "rgba(34, 197, 94, 0.9)"]] : [];
  }, [curEv, showBest]);

  // The class of the move just played, on the square it went to.
  const badge = useMemo(() => {
    const to = uciSquares(lastStep ? lastStep.uci : played?.uci)?.to;
    return to && lastClass ? { square: to, cls: lastClass } : null;
  }, [lastStep, played, lastClass]);

  const boardState = useMemo(
    () => ({ fen: curFen, legalMoves: (legalCache[curFen] || []).map((uci) => ({ uci })) }),
    [curFen, legalCache]
  );

  // The person moves a piece: the move is the game's own next move (just step),
  // the branch's next move (step), or something new (a branch begins or is cut here).
  function playMove(uci) {
    if (!branch && moves[ply]?.uci === uci) {
      go(ply + 1);
      return { ok: true };
    }
    if (branch && branch.steps[branch.at]?.uci === uci) {
      stepForward();
      return { ok: true };
    }
    extend(curFen, [uci]);
    return { ok: true };
  }

  // Adds moves (UCI or SAN, played from the position on the board) to the branch, cutting what came after it.
  function extend(fen, list) {
    const my = ++seq.current;
    const base = branch ? branch.base : ply;
    const keep = branch ? branch.steps.slice(0, branch.at) : [];
    setNotice("");
    fetchLine({ fen, moves: list })
      .then((r) => {
        if (my !== seq.current) return;
        setLegalCache((c) => ({ ...c, [r.fen]: r.legal }));
        setBranch({ base, steps: [...keep, ...r.steps], at: keep.length + r.steps.length });
        if (r.steps.length) playMoveSound(r.steps[r.steps.length - 1].san);
      })
      .catch((e) => {
        if (my !== seq.current) return;
        setNotice(e.message);
        setBoardKey((k) => k + 1); // put the piece back
      });
  }

  // The engine's line up to its k-th move is played out on the board.
  function playLine(k) {
    if (curEv?.pv?.length) extend(curFen, curEv.pv.slice(0, k + 1));
  }

  const pvNumbered = useMemo(() => (curEv?.pv ? numberedSans(curFen, curEv.pv.map((san) => ({ san }))) : []), [curEv, curFen]);
  const branchNumbered = useMemo(() => (branch ? numberedSans(baseFen, branch.steps) : []), [branch, baseFen]);

  async function copy(kind) {
    let text;
    if (kind === "fen") text = curFen;
    else {
      const d = /^(\d{4})\.(\d{2})\.(\d{2})$/.exec(game.date || "");
      text = buildPgn({
        sans: moves.map((m) => m.san),
        startFen: game.startFen,
        result: game.result || "*",
        white: game.white || WHITE_NAME,
        black: game.black || BLACK_NAME,
        date: d ? new Date(Number(d[1]), Number(d[2]) - 1, Number(d[3])) : new Date(),
        event: game.event || "Разбор партии",
      });
    }
    setCopied((await copyText(text)) ? kind : "error");
    setTimeout(() => setCopied(""), 1800);
  }

  const keyPlies = report.key || [];
  const nameOf = (c) => (c === 0 ? game.white || WHITE_NAME : game.black || BLACK_NAME);
  const topSide = orientation === "white" ? "black" : "white";
  const bottomSide = orientation === "white" ? "white" : "black";
  const bestBefore = parentFen ? evCache[parentFen]?.bestSan : "";

  return (
    <div className={`review${inline ? " review--inline" : ""}`}>
      <div className="review__main">
        <div className="review__players review__players--top">
          {nameOf(topSide === "white" ? 0 : 1)}
          <Captured fen={curFen} side={topSide} />
        </div>
        <div className="review__boardrow">
          {settings.showEval && <EvalBar winPct={ev.winPct} label={formatEval(ev)} flipped={orientation === "black"} dim={thinking} />}
          <div className="review__board">
            <Board
              key={boardKey}
              state={boardState}
              orientation={orientation}
              disabled={false}
              lastMove={lastSquares}
              lastMoveColor={lastMoveColor}
              arrows={arrows}
              badge={badge}
              onPlayerMove={playMove}
            />
          </div>
        </div>
        <div className="review__players">
          {nameOf(bottomSide === "white" ? 0 : 1)}
          <Captured fen={curFen} side={bottomSide} />
        </div>

        <div className="review__nav">
          <button type="button" onClick={() => go(0)} disabled={atStart} title="В начало партии (Home)" aria-label="В начало">
            <Icon name="first" />
          </button>
          <button type="button" onClick={stepBack} disabled={atStart} title="Назад (←)" aria-label="Назад">
            <Icon name="prev" />
          </button>
          <button type="button" onClick={stepForward} disabled={atEnd} title="Вперёд (→)" aria-label="Вперёд">
            <Icon name="next" />
          </button>
          <button type="button" onClick={toEnd} disabled={atEnd} title="В конец (End)" aria-label="В конец">
            <Icon name="last" />
          </button>
          <button
            type="button"
            onClick={() => setOrientation((o) => (o === "white" ? "black" : "white"))}
            title="Перевернуть доску"
            aria-label="Перевернуть доску"
          >
            <Icon name="flip" />
          </button>
          <label className="review__toggle">
            <input type="checkbox" checked={showBest} onChange={(e) => setShowBest(e.target.checked)} />
            лучший ход
          </label>
        </div>
        {!branch && <p className="review__hint review__hint--pad">Можно ходить фигурами на доске: так вы проверите свою идею в этой позиции.</p>}

        {settings.showEval && <EvalChart evals={evals} ply={branch ? branch.base : ply} keyPlies={keyPlies} onSelect={go} />}
      </div>

      <aside className="review__side">
        {inline && onBack && (
          <button type="button" className="btn btn--ghost review__back" onClick={onBack}>
            <Icon name="back" size={16} /> К партии
          </button>
        )}
        <div className="review__now card">
          {lastStep ? (
            <BranchNow label={branchNumbered[branch.at - 1]} judge={branchJudge} bestSan={bestBefore} />
          ) : played ? (
            <NowMove move={played} />
          ) : (
            <span className="review__hint">{branch ? "Позиция из партии, с которой начинается ваша ветка" : "Начальная позиция"}</span>
          )}
          <div className="review__line">
            {settings.showEval && <b>{formatEval(ev)}</b>}
            {thinking && <span className="review__thinking"> оцениваю…</span>}
            {ev.terminal ? (
              <span> {terminalText(ev.terminal)}</span>
            ) : (
              pvNumbered.length > 0 &&
              !thinking && (
                <span className="review__pv">
                  {" "}
                  лучшая линия:{" "}
                  {pvNumbered.map((m, i) => (
                    <button
                      key={i}
                      type="button"
                      className="pv-move"
                      onClick={() => playLine(i)}
                      title="Сыграть эту линию до этого хода на доске"
                    >
                      {m.label && <span className="pv-move__no">{m.label}</span>}
                      {m.san}
                    </button>
                  ))}
                  <small> (глубина {ev.depth})</small>
                </span>
              )
            )}
          </div>
        </div>

        {branch && (
          <div className="review__branch card">
            <div className="review__branchhead">
              <h3>Своя ветка</h3>
              <button type="button" className="btn btn--ghost" onClick={() => go(branch.base)} title="Esc">
                <Icon name="back" size={16} /> К партии
              </button>
            </div>
            <div className="review__branchmoves">
              <button
                type="button"
                className={`branch-move${branch.at === 0 ? " branch-move--on" : ""}`}
                onClick={() => setBranch({ ...branch, at: 0 })}
                title="Позиция, с которой начата ветка"
              >
                старт
              </button>
              {branch.steps.map((st, i) => {
                const j =
                  evCache[i === 0 ? baseFen : branch.steps[i - 1].fen] && evCache[st.fen]
                    ? judgeBranchMove(st, evCache[i === 0 ? baseFen : branch.steps[i - 1].fen], evCache[st.fen], sideOf(i === 0 ? baseFen : branch.steps[i - 1].fen))
                    : null;
                return (
                  <button
                    key={i}
                    type="button"
                    className={`branch-move${branch.at === i + 1 ? " branch-move--on" : ""}`}
                    onClick={() => setBranch({ ...branch, at: i + 1 })}
                  >
                    {branchNumbered[i].label && <span className="pv-move__no">{branchNumbered[i].label}</span>}
                    {st.san}
                    {j && <ClassBadge cls={j.cls} size={22} />}
                  </button>
                );
              })}
            </div>
            <div className="review__row">
              <button type="button" className="btn" onClick={() => playLine(0)} disabled={thinking || !curEv?.pv?.length}>
                Сыграть ход движка{curEv?.bestSan && !thinking ? `: ${curEv.bestSan}` : ""}
              </button>
              <button type="button" className="btn btn--ghost" onClick={() => copyBranch(branchNumbered, setCopied)}>
                <Icon name="copy" size={16} /> {copied === "line" ? "Скопировано" : "Ветка"}
              </button>
            </div>
            {notice && <div className="banner banner--error">{notice}</div>}
            <p className="review__hint">Ходите на доске или нажимайте ходы лучшей линии. Движок оценивает каждую позицию, а значок хода показывает, насколько он хорош.</p>
          </div>
        )}
        {!branch && notice && <div className="banner banner--error">{notice}</div>}

        <header className="review__game card">
          <h2>
            {game.white || WHITE_NAME} — {game.black || BLACK_NAME}
          </h2>
          <p className="review__meta">
            {[game.result, game.date, game.event].filter(Boolean).join(" · ")}
          </p>
          {opening && (
            <p className="review__opening">
              <span className="review__eco">{opening.eco}</span> {opening.name}
            </p>
          )}
          <div className="review__copyrow">
            <button type="button" className="btn" onClick={() => copy("fen")} title="Скопировать позицию на доске в формате FEN">
              <Icon name="copy" size={16} /> {copied === "fen" ? "Скопировано" : "Копировать FEN"}
            </button>
            <button type="button" className="btn" onClick={() => copy("pgn")} title="Скопировать партию в формате PGN">
              <Icon name="copy" size={16} /> {copied === "pgn" ? "Скопировано" : "Копировать PGN"}
            </button>
          </div>
          {copied === "error" && <p className="review__hint">Не удалось скопировать: браузер не дал доступ к буферу обмена.</p>}
        </header>

        <MoveList moves={moves} ply={branch ? branch.base : ply} onSelect={go} />

        <Summary report={report} game={game} />

        {keyPlies.length > 0 && (
          <div className="review__key card">
            <h3>Ключевые моменты</h3>
            {keyPlies.map((p) => {
              const m = moves[p - 1];
              return (
                <button key={p} type="button" className="key-moment" onClick={() => go(p)}>
                  <ClassBadge cls={m.class} />
                  {Math.ceil(p / 2)}.{m.color === "black" ? ".." : ""}
                  {m.san} <small>−{m.loss.toFixed(0)}%</small>
                </button>
              );
            })}
          </div>
        )}
      </aside>
    </div>
  );
}

// "1. e4 e5 2. Nf3" of the branch, to the clipboard.
async function copyBranch(numbered, setCopied) {
  setCopied((await copyText(lineText(numbered))) ? "line" : "error");
  setTimeout(() => setCopied(""), 1800);
}

// How the move just played in the branch was judged.
function BranchNow({ label, judge, bestSan }) {
  const c = judge ? CLASSES[judge.cls] : null;
  return (
    <div className="review__playedmove">
      {judge ? <ClassBadge cls={judge.cls} large /> : <span className="badge badge--lg badge--wait" />}
      <span>
        <b>
          {label?.label} {label?.san}
        </b>{" "}
        {c ? (
          <>
            — {c.label}
            {judge.cls !== "best" && <span className="review__loss"> (−{judge.loss.toFixed(0)}%)</span>}
            {bestSan && judge.cls !== "best" && <span className="review__better"> · лучше было {bestSan}</span>}
            <span className="review__why">{c.hint}</span>
          </>
        ) : (
          <span className="review__why">Движок оценивает ход…</span>
        )}
      </span>
    </div>
  );
}

function terminalText(t) {
  if (t === "checkmate") return "мат";
  if (t === "stalemate") return "пат";
  return t;
}

function NowMove({ move }) {
  const c = CLASSES[move.class];
  return (
    <div className="review__playedmove">
      <ClassBadge cls={move.class} large />
      <span>
        <b>
          {Math.ceil(move.ply / 2)}.{move.color === "black" ? ".." : ""}
          {move.san}
        </b>{" "}
        — {c.label}
        {move.bestSan && move.class !== "book" && (
          <span className="review__better"> · лучше было {move.bestSan}</span>
        )}
        <span className="review__why">{c.hint}</span>
      </span>
    </div>
  );
}

function EvalBar({ winPct, label, flipped, dim }) {
  const white = Math.max(0, Math.min(100, winPct));
  return (
    <div className={`evalbar${flipped ? " evalbar--flipped" : ""}${dim ? " evalbar--dim" : ""}`} title={`Вероятность победы белых: ${white.toFixed(0)}%`}>
      <div className="evalbar__white" style={{ height: `${white}%` }} />
      <span className="evalbar__label">{label}</span>
    </div>
  );
}

function Summary({ report, game }) {
  const rows = SUMMARY_ORDER.filter((k) => (report.counts?.[0]?.[k] || 0) + (report.counts?.[1]?.[k] || 0) > 0);
  return (
    <div className="review__summary card">
      <h3>Точность</h3>
      <div className="accuracy">
        <div className="accuracy__box">
          <span className="accuracy__name">{game.white || WHITE_NAME}</span>
          <span className="accuracy__num">{report.accuracy[0].toFixed(1)}</span>
        </div>
        <div className="accuracy__box accuracy__box--black">
          <span className="accuracy__name">{game.black || BLACK_NAME}</span>
          <span className="accuracy__num">{report.accuracy[1].toFixed(1)}</span>
        </div>
      </div>
      <table className="counts">
        <tbody>
          {rows.map((k) => (
            <tr key={k}>
              <td className="counts__n">{report.counts[0]?.[k] || 0}</td>
              <td>
                <ClassBadge cls={k} /> 
                {CLASSES[k].label}
              </td>
              <td className="counts__n">{report.counts[1]?.[k] || 0}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

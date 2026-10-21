import { useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Captured from "../components/Captured.jsx";
import { useMoveSound, useSettings } from "../settings/SettingsContext.jsx";
import { buildPgn } from "../play/game.js";
import EvalChart from "./EvalChart.jsx";
import MoveList from "./MoveList.jsx";
import ClassBadge from "./ClassBadge.jsx";
import EngineLines from "./EngineLines.jsx";
import DionisCard from "./DionisCard.jsx";
import SummaryTab from "./SummaryTab.jsx";
import { PanelNav, PanelTools } from "./PanelNav.jsx";
import { brief as dionisBrief } from "./dionis.js";
import Icon from "../ui/Icon.jsx";
import { CLASSES, formatEval, uciSquares } from "./classes.js";
import { fetchLine } from "./api.js";
import { judgeBranchMove, lineText, numberedSans, sideOf } from "./branch.js";
import { pickArrows } from "./arrows.js";
import { chartMarks, encodeFen, errorCount, momentPlies, moveLabel, reviewUrl, stepMoment } from "./moments.js";
import { copyText } from "./clipboard.js";
import { openingName, openingTitle } from "../openings/names.js";
import "./review.css";

const WHITE_NAME = "Белые";
const BLACK_NAME = "Чёрные";
const AUTO_MS = 1100; // how long the game stays on a move while it is played

// The review of a game (or, with free, the analysis board: a position with no game, where the person
// plays their own moves and the engine judges them).
// inline: shown in place of a game screen (the review of a game just played) instead of on its own
// page; onBack then returns to the game. orientation is the side the board starts from. onNew: leave
// for another game (the review page). startPly: the move a link pointed at. loader: what the analysis
// board shows in its "Загрузить" tab.
export default function ReviewView({ job, inline = false, orientation: startSide = "white", onBack, onNew, startPly = 0, free = false, loader = null }) {
  const { report, game } = job;
  const { evals, moves } = report;
  const last = moves.length;

  const depth = report.options?.depth || 0; // the depth of the review; 0 lets the server choose

  const [ply, setPly] = useState(inline ? last : Math.max(0, Math.min(last, startPly))); // a review of the game just played starts where the game ended
  const [orientation, setOrientation] = useState(startSide);
  const [tab, setTab] = useState("review"); // "review" | "summary" | "load"
  const [auto, setAuto] = useState(false); // the game plays itself, a move at a time
  const { settings, save } = useSettings();
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
  // The engine's best lines of positions (MultiPV), the coach's notes by half-move, and whether a
  // line the coach showed is being played out on its own.
  const [linesCache, setLinesCache] = useState({});
  const [notes, setNotes] = useState({});
  const [demo, setDemo] = useState(false);
  const asking = useRef(new Set()); // requests in flight, by kind and position
  const seq = useRef(0);

  const go = (p) => {
    setDemo(false);
    setAuto(false);
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
    setDemo(false);
    if (!branch) return go(ply - 1);
    if (branch.at > 0) return setBranch({ ...branch, at: branch.at - 1 });
    go(branch.base - 1);
  }
  function stepForward() {
    setDemo(false);
    if (!branch) return go(ply + 1);
    if (branch.at < branch.steps.length) {
      playMoveSound(branch.steps[branch.at].san);
      setBranch({ ...branch, at: branch.at + 1 });
    }
  }
  const toEnd = () => (branch ? setBranch({ ...branch, at: branch.steps.length }) : go(last));
  const atStart = !branch && ply === 0;
  const atEnd = branch ? branch.at === branch.steps.length : ply === last;

  // The moves that cost something: "next mistake" and "previous mistake" walk through them.
  const mistakes = useMemo(() => momentPlies(moves), [moves]);
  const mainPly = branch ? branch.base : ply;
  const prevMistake = stepMoment(mistakes, mainPly, -1);
  const nextMistake = stepMoment(mistakes, mainPly, 1);

  // Play or pause: the game goes on by itself, or, inside a branch, the branch's moves.
  function togglePlay() {
    if (branch) {
      if (demo) return setDemo(false);
      if (branch.at >= branch.steps.length) setBranch({ ...branch, at: 0 });
      return setDemo(true);
    }
    if (auto) return setAuto(false);
    if (ply >= last) setPly(0);
    setAuto(true);
  }
  const playing = branch ? demo : auto;
  const canPlay = branch ? branch.steps.length > 0 : last > 0;

  const flip = () => setOrientation((o) => (o === "white" ? "black" : "white"));
  // "Своя ветка" with no move yet: the board is the person's from here.
  const startBranch = () => {
    setAuto(false);
    setDemo(false);
    setBranch({ base: ply, steps: [], at: 0 });
  };

  useEffect(() => {
    function onKey(e) {
      const tag = e.target.tagName;
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT" || e.altKey || e.ctrlKey || e.metaKey) return;
      if (e.key === "ArrowLeft" && e.shiftKey) {
        if (prevMistake !== null) go(prevMistake);
      } else if (e.key === "ArrowRight" && e.shiftKey) {
        if (nextMistake !== null) go(nextMistake);
      } else if (e.key === "ArrowLeft") stepBack();
      else if (e.key === "ArrowRight") stepForward();
      else if (e.key === "Home") go(0);
      else if (e.key === "End") toEnd();
      else if (e.key === " " && tag !== "BUTTON") togglePlay();
      else if ((e.key === "f" || e.key === "F") && !e.shiftKey) flip();
      else if (e.key === "Escape" && branch) go(branch.base);
      else return;
      e.preventDefault();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const played = !branch && ply > 0 ? moves[ply - 1] : null;
  // The move the coach card is about: the one on the board, or, while one of the coach's own lines is
  // shown, the move that line belongs to.
  const coachPly = !branch ? ply : branch.coachPly || 0;
  // Dionis's short remark on that move, and with it whether his red arrow (the reply that punishes it) is drawn.
  const coachOn = settings.showCoach !== false && !free;
  const remark = useMemo(
    () => dionisBrief({ move: coachPly > 0 ? moves[coachPly - 1] : null, before: evals[Math.max(0, coachPly - 1)], after: evals[coachPly] }),
    [moves, evals, coachPly]
  );

  // Stepping forward one move makes that move's sound; jumps are silent.
  const prevPly = useRef(ply);
  useEffect(() => {
    if (!branch && ply === prevPly.current + 1) playMoveSound(moves[ply - 1].san);
    prevPly.current = ply;
  }, [ply]); // eslint-disable-line react-hooks/exhaustive-deps

  // The game plays itself: the next move after a pause, until the end.
  useEffect(() => {
    if (!auto || branch) return undefined;
    if (ply >= last) {
      setAuto(false);
      return undefined;
    }
    const timer = setTimeout(() => setPly(ply + 1), AUTO_MS);
    return () => clearTimeout(timer);
  }, [auto, ply, branch, last]);

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

  // The engine's best lines of the position on the board come after the evaluation, a moment after the
  // person stops on it (stepping fast through a game must not start a search for every position).
  const linesDepth = Math.min(depth || 14, 14);
  useEffect(() => {
    const own = evCache[curFen];
    if (!own || own.terminal || linesCache[curFen]) return undefined;
    const key = "lines" + curFen;
    const timer = setTimeout(() => {
      if (asking.current.has(key)) return;
      asking.current.add(key);
      fetchLine({ fen: curFen, lines: 3, depth: linesDepth })
        .then((r) => setLinesCache((c) => ({ ...c, [curFen]: r.lines || [] })))
        .catch(() => setLinesCache((c) => ({ ...c, [curFen]: [] }))) // the line of the review stays
        .finally(() => asking.current.delete(key));
    }, 300);
    return () => clearTimeout(timer);
  }, [curFen, evCache, linesCache, linesDepth]);

  // A line the coach showed plays itself, a move at a time, until the end or until the person takes over.
  useEffect(() => {
    if (!demo || !branch) return undefined;
    if (branch.at >= branch.steps.length) {
      setDemo(false);
      return undefined;
    }
    const timer = setTimeout(() => {
      playMoveSound(branch.steps[branch.at].san);
      setBranch({ ...branch, at: branch.at + 1 });
    }, branch.at === 0 ? 700 : 1000);
    return () => clearTimeout(timer);
  }, [demo, branch]); // eslint-disable-line react-hooks/exhaustive-deps

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

  // The engine's three best lines are three arrows (the best thickest, the others thinner and paler, each
  // the more so the worse it is than the best). Before the lines come, the review's own best move is
  // the one arrow. After a move that cost something, with Dionis on, the reply that punishes it is that
  // same best move and is drawn red. "Стрелки движка" (the setting) off leaves only the red one.
  const threat = !!(coachOn && !branch && played && remark.threat);
  const engineLines = linesCache[curFen]?.length ? linesCache[curFen] : null;
  const arrowsOn = settings.engineArrows !== false;
  const engineArrows = useMemo(
    () => (curEv && !curEv.terminal ? pickArrows({ fen: curFen, lines: engineLines, first: curEv, threat, enabled: arrowsOn }) : []),
    [curFen, engineLines, curEv, threat, arrowsOn]
  );

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
    setDemo(false);
    setAuto(false);
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

  // A line of the coach: from the position after basePly half-moves of the game, shown from its start and
  // played out by itself.
  function showLine(basePly, sans, title) {
    const forPly = coachPly; // the move the coach spoke about: its words stay on the screen while the line plays
    const my = ++seq.current;
    setNotice("");
    setAuto(false);
    fetchLine({ fen: evals[basePly].fen, moves: sans })
      .then((r) => {
        if (my !== seq.current) return;
        setLegalCache((c) => ({ ...c, [r.fen]: r.legal }));
        setPly(basePly);
        setBranch({ base: basePly, steps: r.steps, at: 0, title, coachPly: forPly });
        setDemo(true);
      })
      .catch((e) => my === seq.current && setNotice(e.message));
  }
  const branchNumbered = useMemo(() => (branch ? numberedSans(baseFen, branch.steps) : []), [branch, baseFen]);

  function flash(kind, ok) {
    setCopied(ok ? kind : "error");
    setTimeout(() => setCopied(""), 1800);
  }

  async function copy(kind) {
    let text;
    if (kind === "fen") text = curFen;
    else if (kind === "link") {
      if (free) text = `${window.location.origin}${window.location.pathname}#analysis/${encodeFen(curFen)}`;
      else text = reviewUrl(window.location.origin, window.location.pathname, job.id) + (mainPly > 0 ? `/${mainPly}` : "");
    } else {
      const d = /^(\d{4})\.(\d{2})\.(\d{2})$/.exec(game.date || "");
      text = buildPgn({
        sans: free ? (branch ? branch.steps.map((s) => s.san) : []) : moves.map((m) => m.san),
        startFen: free ? evals[0].fen : game.startFen,
        result: free ? "*" : game.result || "*",
        white: game.white || WHITE_NAME,
        black: game.black || BLACK_NAME,
        date: d ? new Date(Number(d[1]), Number(d[2]) - 1, Number(d[3])) : new Date(),
        event: free ? "Доска анализа" : game.event || "Разбор партии",
      });
    }
    flash(kind, await copyText(text));
  }

  const variant = report.variant || game.variant || job.variant; // "chess960" has no book and cannot go to the classical analysis board
  const shareItems = [
    ...(free || job.id
      ? [{ key: "link", label: free ? "Копировать ссылку на позицию" : mainPly > 0 ? "Копировать ссылку на этот ход" : "Копировать ссылку на разбор", icon: "link", keepOpen: true, onClick: () => copy("link") }]
      : []),
    { key: "pgn", label: "Копировать PGN", icon: "copy", keepOpen: true, onClick: () => copy("pgn") },
    { key: "fen", label: "Копировать FEN позиции", icon: "copy", keepOpen: true, onClick: () => copy("fen") },
    ...(onNew && !inline ? [{ key: "new", label: "Разобрать другую партию", icon: "upload", onClick: onNew }] : []),
    ...(free || variant === "chess960" ? [] : [{ key: "analysis", label: "Открыть позицию на доске анализа", icon: "board", onClick: () => (window.location.hash = "analysis/" + encodeFen(curFen)) }]),
  ];

  const nameOf = (c) => (c === 0 ? game.white || WHITE_NAME : game.black || BLACK_NAME);
  const topSide = orientation === "white" ? "black" : "white";
  const bottomSide = orientation === "white" ? "white" : "black";
  const bestBefore = parentFen ? evCache[parentFen]?.bestSan : "";
  const marks = useMemo(() => chartMarks(moves), [moves]);
  const labelOf = (p) => (p === 0 ? "Начало партии" : `${moveLabel(moves[p - 1])} · ${CLASSES[moves[p - 1].class]?.label || ""}`);
  const whiteName = nameOf(0);
  const blackName = nameOf(1);
  const errs = [errorCount(report.counts?.[0]), errorCount(report.counts?.[1])];
  const navProps = { atStart, atEnd, playing, canPlay, onFirst: () => go(0), onPrev: stepBack, onNext: stepForward, onLast: toEnd, onPlay: togglePlay };

  const tabs = free
    ? [
        { id: "review", label: "Анализ" },
        { id: "load", label: "Загрузить" },
      ]
    : [
        { id: "review", label: "Разбор" },
        { id: "summary", label: "Итог" },
      ];
  const shownTab = tabs.some((t) => t.id === tab) ? tab : "review";

  return (
    <div className={`review${inline ? " review--inline" : ""}${free ? " review--free" : ""}`}>
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
              engineArrows={engineArrows}
              badge={badge}
              onPlayerMove={playMove}
            />
          </div>
        </div>
        <div className="review__players">
          {nameOf(bottomSide === "white" ? 0 : 1)}
          <Captured fen={curFen} side={bottomSide} />
        </div>
        {/* the panel is hidden in the "board in the centre" mode: these keep the stepping under the board */}
        <PanelNav {...navProps} className="rnav--board" />
      </div>

      <aside className="review__side rv card">
        <header className="rv__head">
          <div className="rv__titlerow">
            {free ? (
              <h2 className="rv__title">Доска анализа</h2>
            ) : (
              <p className="rv__meta">
                {[game.result, game.date, game.event, variant === "chess960" ? "Chess960" : ""].filter(Boolean).join(" · ")}
              </p>
            )}
            {!free && opening && (
              <p className="rv__opening" title={openingTitle(opening)}>
                <span className="review__eco">{opening.eco}</span> {openingName(opening)}
              </p>
            )}
            {inline && onBack && (
              <button type="button" className="btn btn--small btn--ghost" onClick={onBack}>
                <Icon name="back" size={14} /> К партии
              </button>
            )}
          </div>
          {free && <p className="rv__meta">Свободная доска: ходите любыми фигурами, движок оценивает позицию и каждый ход.</p>}
          {!free && variant === "chess960" && (
            // Chess960: no opening names or theory moves; the engine searches positions without castling rights
            <p className="rv__meta" style={{ WebkitLineClamp: "unset" }}>
              Chess960: дебютной теории нет. Пока рокировка возможна, движок её не видит и лучшим ходом не назовёт.
            </p>
          )}
          {!free && (
            <div className="rv__acc" aria-label="Точность игроков">
              <span className="rv__pill" title={`Точность ${whiteName}`}>
                <span className="rv__pillname">{whiteName}</span>
                <b>{report.accuracy[0].toFixed(1)}</b>
              </span>
              <span className="rv__pill rv__pill--black" title={`Точность ${blackName}`}>
                <span className="rv__pillname">{blackName}</span>
                <b>{report.accuracy[1].toFixed(1)}</b>
              </span>
            </div>
          )}
        </header>

        <div className="rv__tabs" role="tablist">
          {tabs.map((t) => (
            <button key={t.id} type="button" role="tab" aria-selected={shownTab === t.id} className={`rv__tab${shownTab === t.id ? " rv__tab--on" : ""}`} onClick={() => setTab(t.id)}>
              {t.label}
              {t.id === "summary" && errs[0] + errs[1] > 0 && (
                <small className="rv__tabnote" title={`Неточностей, ошибок и зевков: ${whiteName} ${errs[0]}, ${blackName} ${errs[1]}`}>
                  {errs[0]} / {errs[1]}
                </small>
              )}
            </button>
          ))}
          <label className="rv__arrow" title="Стрелки движка: три лучшие линии на доске (Настройки, Игровой процесс, Анализ)">
            <input type="checkbox" checked={arrowsOn} onChange={(e) => save({ ...settings, engineArrows: e.target.checked })} />
            стрелки
          </label>
        </div>

        <div className="rv__body">
          {shownTab === "summary" && !free && (
            <SummaryTab
              report={report}
              game={game}
              whiteName={whiteName}
              blackName={blackName}
              ply={mainPly}
              showEval={settings.showEval}
              labelOf={labelOf}
              onGo={go}
              onOpen={(p) => {
                go(p);
                setTab("review");
              }}
            />
          )}

          {shownTab === "load" && free && <div className="rv__load">{loader}</div>}

          {shownTab === "review" && (
            <>
              <div className="rv__move">
                {lastStep ? (
                  <BranchNow label={branchNumbered[branch.at - 1]} judge={branchJudge} bestSan={bestBefore} />
                ) : played ? (
                  <NowMove move={played} hint={!coachOn} />
                ) : (
                  <span className="review__hint">
                    {branch
                      ? free
                        ? "Сходите фигурой на доске или нажмите ход линии движка."
                        : "Позиция из партии, с которой начинается ваша ветка"
                      : "Начальная позиция"}
                  </span>
                )}
                {(ev.terminal || thinking) && (
                  <span className="rv__evalnote">
                    {settings.showEval && <b>{formatEval(ev)}</b>}
                    {thinking && <span className="review__thinking"> оцениваю…</span>}
                    {ev.terminal && <span> {terminalText(ev.terminal)}</span>}
                  </span>
                )}
              </div>

              {coachOn && (!branch || branch.coachPly > 0) && (
                <DionisCard
                  jobId={job.id}
                  ply={coachPly}
                  move={coachPly > 0 ? moves[coachPly - 1] : null}
                  brief={remark}
                  note={notes[coachPly] || null}
                  onNote={(p, n) => setNotes((s) => ({ ...s, [p]: n }))}
                  onShow={showLine}
                  onHide={() => save({ ...settings, showCoach: false })}
                />
              )}

              {!ev.terminal && !thinking && (
                <EngineLines
                  fen={curFen}
                  lines={engineLines}
                  first={curEv}
                  showEval={settings.showEval}
                  onPlay={(pv, k) => extend(curFen, pv.slice(0, k + 1))}
                />
              )}

              {branch && (
                <div className="review__branch">
                  <div className="review__branchhead">
                    <h3>{branch.title || "Своя ветка"}</h3>
                    <button type="button" className="btn btn--small btn--ghost" onClick={() => go(branch.base)} title="Esc">
                      <Icon name="back" size={14} /> {free ? "С начала" : "К партии"}
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
                          {j && <ClassBadge cls={j.cls} size={20} />}
                        </button>
                      );
                    })}
                  </div>
                  <div className="review__row">
                    <button type="button" className="btn btn--small" onClick={() => playLine(0)} disabled={thinking || !curEv?.pv?.length}>
                      Сыграть ход движка{curEv?.bestSan && !thinking ? `: ${curEv.bestSan}` : ""}
                    </button>
                    <button type="button" className="btn btn--small btn--ghost" onClick={() => copyBranch(branchNumbered, flash)}>
                      <Icon name="copy" size={14} /> {copied === "line" ? "Скопировано" : "Ходы"}
                    </button>
                  </div>
                  {notice && <div className="banner banner--error">{notice}</div>}
                  {branch.steps.length === 0 && <p className="review__hint">Ходите на доске или нажимайте ходы линий движка.</p>}
                </div>
              )}
              {!branch && notice && <div className="banner banner--error">{notice}</div>}
              {!branch && free && <p className="review__hint rv__freehint">Сходите любой фигурой: откроется ветка, движок оценит каждый ход.</p>}

              {!free && settings.showEval && evals.length > 1 && (
                <EvalChart evals={evals} ply={mainPly} marks={marks} onSelect={go} labelOf={labelOf} />
              )}
              {!free && <MoveList moves={moves} ply={mainPly} onSelect={go} />}
              {copied === "error" && <p className="review__hint">Не удалось скопировать: браузер не дал доступ к буферу обмена.</p>}
            </>
          )}
        </div>

        <footer className="rv__foot">
          <PanelNav {...navProps} />
          <PanelTools
            hideMoments={free}
            canPrevMoment={prevMistake !== null}
            canNextMoment={nextMistake !== null}
            onPrevMoment={() => go(prevMistake)}
            onNextMoment={() => go(nextMistake)}
            onFlip={flip}
            onBranch={() => (branch ? go(branch.base) : startBranch())}
            branchOn={!!branch}
            onLink={() => copy("link")}
            share={shareItems}
            copied={copied}
          />
        </footer>
      </aside>
    </div>
  );
}

// "1. e4 e5 2. Nf3" of the branch, to the clipboard.
async function copyBranch(numbered, flash) {
  flash("line", await copyText(lineText(numbered)));
}

// How the move just played in the branch was judged.
function BranchNow({ label, judge, bestSan }) {
  const c = judge ? CLASSES[judge.cls] : null;
  return (
    <div className="review__playedmove">
      {judge ? <ClassBadge cls={judge.cls} size={34} /> : <span className="badge badge--lg badge--wait" />}
      <span className="rv__movetext">
        <b>
          {label?.label} {label?.san}
        </b>{" "}
        {c ? (
          <>
            — {c.label}
            {judge.cls !== "best" && <span className="review__loss"> (−{judge.loss.toFixed(0)}%)</span>}
            {bestSan && judge.cls !== "best" && <span className="review__better"> · лучше было {bestSan}</span>}
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

function NowMove({ move, hint }) {
  const c = CLASSES[move.class];
  return (
    <div className="review__playedmove">
      <ClassBadge cls={move.class} size={34} />
      <span className="rv__movetext">
        <b>{moveLabel(move)}</b> — {c.label}
        {move.bestSan && move.class !== "book" && <span className="review__better"> · лучше было {move.bestSan}</span>}
        {hint && <span className="review__why">{c.hint}</span>}
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

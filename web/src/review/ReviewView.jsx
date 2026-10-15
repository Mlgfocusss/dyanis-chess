import { useEffect, useMemo, useRef, useState } from "react";
import ThemedBoard from "../components/ThemedBoard.jsx";
import { useMoveSound, useSettings } from "../settings/SettingsContext.jsx";
import EvalChart from "./EvalChart.jsx";
import MoveList from "./MoveList.jsx";
import ClassBadge from "./ClassBadge.jsx";
import Icon from "../ui/Icon.jsx";
import { CLASSES, SUMMARY_ORDER, formatEval, uciSquares } from "./classes.js";
import "./review.css";

const WHITE_NAME = "Белые";
const BLACK_NAME = "Чёрные";

export default function ReviewView({ job }) {
  const { report, game } = job;
  const { evals, moves } = report;
  const last = moves.length;

  const [ply, setPly] = useState(0);
  const [orientation, setOrientation] = useState("white");
  const [showBest, setShowBest] = useState(true);
  const { settings } = useSettings();
  const playMoveSound = useMoveSound();

  const go = (p) => setPly(Math.max(0, Math.min(last, p)));

  useEffect(() => {
    function onKey(e) {
      const tag = e.target.tagName;
      if (tag === "TEXTAREA" || tag === "INPUT" || e.altKey || e.ctrlKey || e.metaKey) return;
      if (e.key === "ArrowLeft") setPly((p) => Math.max(0, p - 1));
      else if (e.key === "ArrowRight") setPly((p) => Math.min(last, p + 1));
      else if (e.key === "Home") setPly(0);
      else if (e.key === "End") setPly(last);
      else return;
      e.preventDefault();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [last]);

  const ev = evals[ply];
  const played = ply > 0 ? moves[ply - 1] : null;

  // Stepping forward one move makes that move's sound; jumps are silent.
  const prevPly = useRef(ply);
  useEffect(() => {
    if (ply === prevPly.current + 1) playMoveSound(moves[ply - 1].san);
    prevPly.current = ply;
  }, [ply]); // eslint-disable-line react-hooks/exhaustive-deps

  // The opening is the latest name reached up to the current ply.
  const opening = useMemo(() => {
    for (let i = ply - 1; i >= 0; i--) if (moves[i].opening) return moves[i].opening;
    return null;
  }, [moves, ply]);

  const squareStyles = useMemo(() => {
    const s = {};
    const lm = settings.highlightLastMove ? uciSquares(played?.uci) : null;
    if (lm) {
      const color = played.class === "blunder" || played.class === "mistake"
        ? "rgba(251, 146, 60, 0.6)"
        : "rgba(250, 204, 21, 0.45)";
      s[lm.from] = { backgroundColor: color };
      s[lm.to] = { backgroundColor: color };
    }
    return s;
  }, [played, settings.highlightLastMove]);

  const arrows = useMemo(() => {
    const b = uciSquares(ev?.bestUci);
    return showBest && b ? [[b.from, b.to, "rgba(34, 197, 94, 0.9)"]] : [];
  }, [ev, showBest]);

  const keyPlies = report.key || [];
  const nameOf = (c) => (c === 0 ? game.white || WHITE_NAME : game.black || BLACK_NAME);

  return (
    <div className="review">
      <div className="review__main">
        <div className="review__players review__players--top">
          {nameOf(orientation === "white" ? 1 : 0)}
        </div>
        <div className="review__boardrow">
          <EvalBar winPct={ev.winPct} label={formatEval(ev)} flipped={orientation === "black"} />
          <div className="review__board">
            <ThemedBoard
              id="ReviewBoard"
              position={ev.fen}
              orientation={orientation}
              arePiecesDraggable={false}
              customSquareStyles={squareStyles}
              customArrows={arrows}
            />
          </div>
        </div>
        <div className="review__players">{nameOf(orientation === "white" ? 0 : 1)}</div>

        <div className="review__nav">
          <button type="button" onClick={() => go(0)} disabled={ply === 0} title="В начало (Home)" aria-label="В начало">
            <Icon name="first" />
          </button>
          <button type="button" onClick={() => go(ply - 1)} disabled={ply === 0} title="Назад (←)" aria-label="Назад">
            <Icon name="prev" />
          </button>
          <button type="button" onClick={() => go(ply + 1)} disabled={ply === last} title="Вперёд (→)" aria-label="Вперёд">
            <Icon name="next" />
          </button>
          <button type="button" onClick={() => go(last)} disabled={ply === last} title="В конец (End)" aria-label="В конец">
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

        <EvalChart evals={evals} ply={ply} keyPlies={keyPlies} onSelect={go} />
      </div>

      <aside className="review__side">
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
        </header>

        <div className="review__now card">
          {played ? <NowMove move={played} /> : <span className="review__hint">Начальная позиция</span>}
          <div className="review__line">
            <b>{formatEval(ev)}</b>
            {ev.terminal ? (
              <span> {terminalText(ev.terminal)}</span>
            ) : (
              ev.pv?.length > 0 && (
                <span>
                  {" "}
                  лучшая линия: {ev.pv.join(" ")}
                  <small> (глубина {ev.depth})</small>
                </span>
              )
            )}
          </div>
        </div>

        <MoveList moves={moves} ply={ply} onSelect={go} />

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
      </span>
    </div>
  );
}

function EvalBar({ winPct, label, flipped }) {
  const white = Math.max(0, Math.min(100, winPct));
  return (
    <div className={`evalbar${flipped ? " evalbar--flipped" : ""}`} title={`Вероятность победы белых: ${white.toFixed(0)}%`}>
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

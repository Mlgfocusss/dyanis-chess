import { useCallback, useEffect, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Avatar from "../auth/Avatar.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { START_FEN } from "../play/game.js";
import PuzzleTabs from "./PuzzleTabs.jsx";
import { fetchRushBest, fetchRushPuzzle, sendRushResult } from "./api.js";
import { judgeMove, legalFor, squaresOf } from "./puzzle.js";
import { RUSH_MODES, clockText, runSummary, targetRating } from "./rush.js";
import "./puzzles.css";

const MODE_KEY = "dyanis.rushMode";
const INTRO_MS = 350; // pause before the opponent's first move
const REPLY_MS = 300; // pause before the opponent answers
const NEXT_MS = 250; // pause between a solved puzzle and the next
const TICK_MS = 200;

function savedMode() {
  try {
    const m = localStorage.getItem(MODE_KEY);
    return RUSH_MODES[m] ? m : "3min";
  } catch {
    return "3min";
  }
}

// The tactics storm: puzzles from easy to hard, one after another, until the clock runs out or the
// first mistake. Practice: the puzzle rating does not change; the best score of each length is kept.
export default function RushPage() {
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();

  const [mode, setMode] = useState(savedMode);
  const [phase, setPhase] = useState("idle"); // idle | starting | intro | solving | reply | between | over
  const [puzzle, setPuzzle] = useState(null);
  const [fen, setFen] = useState(START_FEN);
  const [idx, setIdx] = useState(0);
  const [last, setLast] = useState(null);
  const [badge, setBadge] = useState(null);
  const [score, setScore] = useState(0);
  const [left, setLeft] = useState(RUSH_MODES[mode].ms);
  const [best, setBest] = useState({}); // { "3min": { best, runs } }
  const [over, setOver] = useState(null); // { reason, score, record, expected }
  const [error, setError] = useState("");

  const phaseRef = useRef("idle");
  const scoreRef = useRef(0);
  const run = useRef(0); // which run the callbacks belong to
  const deadline = useRef(0);
  const clock = useRef(null);
  const timers = useRef([]);
  const seen = useRef([]);
  const prefetch = useRef(null);
  const bestRef = useRef(best);
  bestRef.current = best;

  const setPh = useCallback((p) => {
    phaseRef.current = p;
    setPhase(p);
  }, []);
  const later = useCallback((fn, ms) => {
    timers.current.push(setTimeout(fn, ms));
  }, []);
  const stopTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
    clearInterval(clock.current);
  }, []);
  useEffect(() => stopTimers, [stopTimers]);

  const sound = useCallback(
    (kind) => {
      if (settings.sound) playSound(kind, settings.soundTheme);
    },
    [settings.sound, settings.soundTheme]
  );

  useEffect(() => {
    if (status !== "ready" || !user) return;
    fetchRushBest().then(setBest, () => {});
  }, [status, user]);

  function chooseMode(m) {
    if (phaseRef.current !== "idle" && phaseRef.current !== "over") return;
    setMode(m);
    setLeft(RUSH_MODES[m].ms);
    try {
      localStorage.setItem(MODE_KEY, m);
    } catch {
      // not remembered
    }
  }

  function finish(reason, expected = "") {
    if (phaseRef.current === "over") return;
    run.current++; // whatever is still waiting belongs to a run that is over
    stopTimers();
    setPh("over");
    const final = scoreRef.current;
    const sum = runSummary(reason, final, bestRef.current[mode]?.best || 0);
    setOver({ reason, score: final, record: sum.record, headline: sum.headline, expected });
    setBadge(null);
    if (reason === "time") setLeft(0);
    sendRushResult(mode, final).then(setBest, () => {});
  }

  // Puts a puzzle on the board: the opponent plays the move that makes it, then the person solves it.
  function begin(p) {
    const mine = run.current;
    seen.current.push(p.id);
    setPuzzle(p);
    setFen(p.fen);
    setIdx(0);
    setLast(null);
    setBadge(null);
    setPh("intro");
    // the next puzzle is asked for while this one is solved
    prefetch.current = fetchRushPuzzle(targetRating(scoreRef.current + 1), seen.current).catch(() => null);
    later(() => {
      if (mine !== run.current) return;
      const first = p.steps[0];
      setFen(first.fen);
      setLast(squaresOf(first.uci));
      sound(soundKindForSan(first.san));
      setIdx(1);
      setPh("solving");
    }, INTRO_MS);
  }

  async function start() {
    setError("");
    setOver(null);
    setPh("starting");
    const mine = ++run.current;
    stopTimers();
    scoreRef.current = 0;
    seen.current = [];
    setScore(0);
    try {
      if (!user) await guest(); // a guest account keeps the records
      const d = await fetchRushPuzzle(targetRating(0));
      if (mine !== run.current) return;
      deadline.current = Date.now() + RUSH_MODES[mode].ms;
      setLeft(RUSH_MODES[mode].ms);
      clock.current = setInterval(() => {
        const rest = deadline.current - Date.now();
        setLeft(rest);
        if (rest <= 0) finish("time");
      }, TICK_MS);
      begin(d.puzzle);
    } catch (e) {
      if (mine !== run.current) return;
      setError(e.message);
      setPh("idle");
    }
  }

  function solved() {
    const mine = run.current;
    scoreRef.current += 1;
    setScore(scoreRef.current);
    setPh("between");
    later(async () => {
      let d = await prefetch.current;
      if (mine !== run.current) return;
      if (!d) {
        try {
          d = await fetchRushPuzzle(targetRating(scoreRef.current), seen.current);
        } catch (e) {
          if (mine === run.current) {
            setError(e.message);
            finish("time");
          }
          return;
        }
      }
      if (mine === run.current) begin(d.puzzle);
    }, NEXT_MS);
  }

  function onPlayerMove(uci) {
    if (phaseRef.current !== "solving") return { ok: false };
    const j = judgeMove(puzzle, idx, uci);
    if (j.kind === "wrong") {
      setBadge({ square: uci.slice(2, 4), cls: "blunder" });
      finish("mistake", puzzle.steps[idx].san);
      return { ok: false };
    }
    setFen(j.fen);
    setLast(squaresOf(uci));
    sound(soundKindForSan(j.san));
    if (j.done) {
      solved();
      return { ok: true };
    }
    const mine = run.current;
    const reply = puzzle.steps[idx + 1];
    setIdx(idx + 1);
    setPh("reply");
    later(() => {
      if (mine !== run.current) return;
      setFen(reply.fen);
      setLast(squaresOf(reply.uci));
      sound(soundKindForSan(reply.san));
      if (idx + 2 >= puzzle.steps.length) {
        solved(); // the line ends with the opponent's move
      } else {
        setIdx(idx + 2);
        setPh("solving");
      }
    }, REPLY_MS);
    return { ok: true };
  }

  if (status === "unavailable") {
    return (
      <div className="room puzzles">
        <div className="banner banner--error">Задачи недоступны: сервер запущен без базы данных.</div>
      </div>
    );
  }

  const running = phase !== "idle" && phase !== "over";
  const mySide = settings.whiteBottom ? "white" : puzzle?.orientation || "white";
  const modeBest = best[mode]?.best || 0;
  const low = running && left < 10_000;

  return (
    <div className="room puzzles rush">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar puzzles__top">
            <span className="puzzles__ico">
              <Icon name="bolt" size={22} />
            </span>
            <div className="pbar__who">
              <span className="pbar__name">Тактический штурм</span>
              <span className="pbar__sub">{running || over ? `Решено: ${score}` : "Задачи от лёгких к сложным, до первой ошибки"}</span>
            </div>
            <span className={`rush__clock${low ? " rush__clock--low" : ""}`} aria-label="Осталось времени">
              {clockText(left)}
            </span>
          </div>
          <div className="room__board">
            <Board
              state={{ fen, legalMoves: phase === "solving" ? legalFor(puzzle, idx) : [] }}
              orientation={mySide}
              disabled={phase !== "solving"}
              lastMove={last}
              badge={badge}
              onPlayerMove={onPlayerMove}
            />
          </div>
          <div className="pbar">
            <Avatar user={user} size={36} />
            <div className="pbar__who">
              <span className="pbar__name">
                {user?.name || "Вы"}
                <span className="pbar__you">вы</span>
              </span>
              <span className="pbar__sub">{puzzle && running ? (mySide === "white" ? "Белые" : "Чёрные") : RUSH_MODES[mode].label}</span>
            </div>
            <span className="puzzles__rating" title="Решено в этой серии">
              {score}
            </span>
          </div>
        </div>

        <aside className="room__side">
          <PuzzleTabs mode="rush" />

          {error && <div className="banner banner--error">{error}</div>}

          {!running && (
            <section className={`card room__status puzzles__state${over ? (over.record ? " puzzles__state--ok" : " puzzles__state--warn") : ""}`} aria-live="polite">
              {over ? (
                <>
                  <p className="room__state">{over.headline}</p>
                  {over.record && <p className="room__meta">Новый рекорд за {RUSH_MODES[mode].label}!</p>}
                  {over.reason === "mistake" && over.expected && <p className="room__meta">Верный ход был {over.expected}.</p>}
                </>
              ) : (
                <>
                  <p className="room__state">Решайте задачи как можно дольше</p>
                  <p className="room__meta">
                    Первые задачи лёгкие, каждая следующая труднее. Серия кончается первой ошибкой или когда выйдет время. Рейтинг задач не меняется.
                  </p>
                </>
              )}
            </section>
          )}

          {running && (
            <section className="card room__status" aria-live="polite">
              <p className="room__state">{phase === "starting" ? "Готовлю задачу…" : `Решено: ${score}`}</p>
              <p className="room__meta">Одна ошибка заканчивает серию.</p>
            </section>
          )}

          <section className="card room__actions">
            <Segmented
              label="Длина серии"
              value={mode}
              onChange={chooseMode}
              options={Object.entries(RUSH_MODES).map(([id, m]) => [id, m.label])}
            />
            {!running ? (
              <button type="button" className="btn btn--primary" onClick={start} disabled={status !== "ready"}>
                <Icon name="bolt" size={16} /> {over ? "Играть снова" : "Начать"}
              </button>
            ) : (
              <button type="button" className="btn btn--ghost" onClick={() => finish("quit")}>
                Закончить серию
              </button>
            )}
          </section>

          <section className="card puzzles__you">
            <h2>Ваш рекорд · {RUSH_MODES[mode].label}</h2>
            <p className="puzzles__big">{modeBest}</p>
            <p className="room__meta">
              Серий сыграно: {best[mode]?.runs || 0}
              {user?.kind === "guest" ? ". Войдите через Google, чтобы рекорды не потерялись." : ""}
            </p>
          </section>
        </aside>
      </div>
    </div>
  );
}

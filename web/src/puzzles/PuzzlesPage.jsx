import { useCallback, useEffect, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Avatar from "../auth/Avatar.jsx";
import Captured from "../components/Captured.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import Icon from "../ui/Icon.jsx";
import { fetchPuzzle, fetchThemes, sendDailyResult, sendResult } from "./api.js";
import { dayLabel, hintSquare, judgeMove, legalFor, signed, squaresOf } from "./puzzle.js";
import { ideaThemes, themeName } from "./themes.js";
import PuzzleTabs, { chooseMode } from "./PuzzleTabs.jsx";
import "./puzzles.css";

const THEME_KEY = "dyanis.puzzleTheme";
const INTRO_MS = 800; // pause before the opponent's first move
const REPLY_MS = 500; // pause before the opponent answers
const SHOW_MS = 800; // between the moves of the shown solution

function savedTheme() {
  try {
    return localStorage.getItem(THEME_KEY) || "";
  } catch {
    return "";
  }
}

// "#puzzles" is the rated puzzles, "#puzzles/themes" and "#puzzles/daily" the other two tabs,
// "#puzzles/<id>" one particular puzzle (in the first tab).
function fromHash() {
  const [page, part] = window.location.hash.slice(1).split("/");
  if (page !== "puzzles" || !part || part === "rush" || part === "duel") return { mode: "rated", id: "" }; // those two are other pages (RushPage)
  if (part === "themes" || part === "daily") return { mode: part, id: "" };
  return { mode: "rated", id: part };
}

export default function PuzzlesPage() {
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();

  const [mode, setMode] = useState(() => fromHash().mode); // rated | themes | daily
  const [theme, setTheme] = useState(savedTheme);
  const [themes, setThemes] = useState([]);
  const [puzzle, setPuzzle] = useState(null);
  const [daily, setDaily] = useState(null); // { date, status } while the puzzle of the day is on the board
  const [you, setYou] = useState(null); // { rating, attempts, solved }
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  // the state of the puzzle being solved
  const [fen, setFen] = useState("");
  const [idx, setIdx] = useState(0); // the step to play next
  const [phase, setPhase] = useState("loading"); // loading | intro | solving | reply | solved | shown
  const [failed, setFailed] = useState(false); // a wrong move or the solution was shown: the first try is lost
  const [last, setLast] = useState(null);
  const [badge, setBadge] = useState(null);
  const [hint, setHint] = useState(null);
  const [result, setResult] = useState(null); // the server's word on the first try

  const [startedAt, setStartedAt] = useState(0); // when the puzzle appeared (for the timer)
  const [elapsed, setElapsed] = useState(0); // seconds spent on it
  const timers = useRef([]);
  const reported = useRef(false);
  const request = useRef(0);

  const later = useCallback((fn, ms) => {
    timers.current.push(setTimeout(fn, ms));
  }, []);
  const clearTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
  }, []);
  useEffect(() => clearTimers, [clearTimers]);

  const sound = useCallback(
    (kind) => {
      if (settings.sound) playSound(kind, settings.soundTheme);
    },
    [settings.sound, settings.soundTheme]
  );

  // The timer of the puzzle (settings → Игровой процесс): seconds since it appeared, stopping when it is done.
  const doneNow = phase === "solved" || phase === "shown";
  useEffect(() => {
    if (!settings.puzzleTimer || !startedAt || doneNow || phase === "loading") return undefined;
    const t = setInterval(() => setElapsed(Math.floor((Date.now() - startedAt) / 1000)), 500);
    return () => clearInterval(t);
  }, [settings.puzzleTimer, startedAt, doneNow, phase]);

  // Starts a puzzle: the board shows the position, then the opponent plays the move that makes it.
  const start = useCallback(
    (p) => {
      clearTimers();
      reported.current = false;
      setStartedAt(Date.now());
      setElapsed(0);
      setPuzzle(p);
      setFen(p.fen);
      setIdx(0);
      setPhase("intro");
      setFailed(false);
      setLast(null);
      setBadge(null);
      setHint(null);
      setResult(null);
      later(() => {
        const first = p.steps[0];
        setFen(first.fen);
        setLast(squaresOf(first.uci));
        sound(soundKindForSan(first.san));
        setIdx(1);
        setPhase("solving");
      }, INTRO_MS);
    },
    [clearTimers, later, sound]
  );

  const load = useCallback(
    async (opts) => {
      const mine = ++request.current;
      setLoading(true);
      setError("");
      try {
        if (!user) await guest(); // a guest account keeps the rating
        const d = await fetchPuzzle({ level: settings.puzzleLevel, ...opts });
        if (mine !== request.current) return;
        setYou(d.you || null);
        setDaily(d.daily || null);
        start(d.puzzle);
      } catch (e) {
        if (mine === request.current) setError(e.message);
      } finally {
        if (mine === request.current) setLoading(false);
      }
    },
    [user, guest, start, settings.puzzleLevel]
  );

  useEffect(() => {
    if (status !== "ready") {
      if (status === "unavailable") setLoading(false);
      return;
    }
    const { mode: first, id } = fromHash();
    if (first === "themes") {
      // the first theme of the list stands in when none was chosen yet
      fetchThemes().then(
        (ts) => {
          setThemes(ts);
          const th = theme || ts[0]?.id || "";
          setTheme(th);
          load({ theme: th });
        },
        () => load({ theme })
      );
      return;
    }
    load(first === "daily" ? { daily: true } : { id });
    fetchThemes().then(setThemes, () => {});
  }, [status]); // eslint-disable-line react-hooks/exhaustive-deps

  // The address is the one source of the tab: the tabs, the menu on the left and a typed
  // link all change the hash, and this follows it.
  function applyMode(m) {
    if (m === mode) return;
    setMode(m);
    if (m === "daily") load({ daily: true });
    else if (m === "themes") {
      const th = theme || themes[0]?.id || "";
      setTheme(th);
      load({ theme: th });
    } else load({});
  }

  // One listener for the life of the page: the menu's own hashchange handler re-renders in the
  // middle of the event, and a listener added by that render would miss the event in progress.
  const apply = useRef(applyMode);
  apply.current = applyMode;
  useEffect(() => {
    const onHash = () => {
      if (window.location.hash.startsWith("#puzzles")) apply.current(fromHash().mode);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  function chooseTheme(t) {
    setTheme(t);
    try {
      localStorage.setItem(THEME_KEY, t);
    } catch {
      // private window: the choice is just not remembered
    }
    load({ theme: t });
  }

  // The first try is reported once: a wrong move or the shown solution is a failure. Only the
  // rated puzzles move the rating; the others are practice (the puzzle of the day keeps its record).
  function report(p, solved) {
    if (reported.current) return;
    reported.current = true;
    if (mode === "themes") return;
    if (mode === "daily") {
      sendDailyResult(solved).then(
        (r) => setDaily((d) => (d ? { ...d, status: r.status } : d)),
        (e) => setError(e.message)
      );
      return;
    }
    sendResult(p.id, solved).then(
      (r) => {
        setResult(r);
        setYou(r.you);
      },
      (e) => setError(e.message)
    );
  }

  function flash(square, cls, ms) {
    setBadge({ square, cls });
    later(() => setBadge(null), ms);
  }

  function onPlayerMove(uci) {
    if (phase !== "solving") return { ok: false };
    const to = uci.slice(2, 4);
    const j = judgeMove(puzzle, idx, uci);
    if (j.kind === "wrong") {
      flash(to, "blunder", 1200);
      setHint(null);
      if (!failed) {
        setFailed(true);
        report(puzzle, false);
      }
      return { ok: false };
    }
    setFen(j.fen);
    setLast(squaresOf(uci));
    setHint(null);
    sound(soundKindForSan(j.san));
    flash(to, "best", 900);
    if (j.done) {
      setPhase("solved");
      if (!failed) report(puzzle, true);
      return { ok: true };
    }
    // the opponent answers
    const reply = puzzle.steps[idx + 1];
    setIdx(idx + 1);
    setPhase("reply");
    later(() => {
      setFen(reply.fen);
      setLast(squaresOf(reply.uci));
      sound(soundKindForSan(reply.san));
      if (idx + 2 >= puzzle.steps.length) {
        setPhase("solved"); // the line ends with the opponent's move
        if (!failed) report(puzzle, true);
      } else {
        setIdx(idx + 2);
        setPhase("solving");
      }
    }, REPLY_MS);
    return { ok: true };
  }

  function showHint() {
    setHint(hintSquare(puzzle, idx));
  }

  function showSolution() {
    clearTimers();
    setHint(null);
    setBadge(null);
    setPhase("shown");
    if (!failed) {
      setFailed(true);
      report(puzzle, false);
    }
    puzzle.steps.slice(idx).forEach((s, i) => {
      later(() => {
        setFen(s.fen);
        setLast(squaresOf(s.uci));
        sound(soundKindForSan(s.san));
      }, SHOW_MS * (i + 1));
    });
  }

  if (status === "unavailable") {
    return (
      <div className="room puzzles">
        <div className="banner banner--error">Задачи недоступны: сервер запущен без базы данных.</div>
      </div>
    );
  }
  if (!puzzle) {
    return (
      <div className="room puzzles puzzles--loading">
        {error ? <div className="banner banner--error">{error}</div> : "Загружаю задачу…"}
      </div>
    );
  }

  const mySide = puzzle.orientation;
  const viewSide = settings.whiteBottom ? "white" : mySide; // "white always at the bottom"
  const myName = user?.name || "Вы";
  const finished = phase === "solved" || phase === "shown";
  const boardState = { fen, legalMoves: phase === "solving" ? legalFor(puzzle, idx) : [] };

  let headline;
  let tone = "";
  if (loading) headline = "Загружаю задачу…";
  else if (phase === "intro") headline = "Соперник делает ход…";
  else if (phase === "solved") {
    headline = failed ? "Решено, но с ошибкой" : "Задача решена!";
    tone = failed ? "warn" : "ok";
  } else if (phase === "shown") {
    headline = "Вот решение";
    tone = "warn";
  } else if (failed) {
    headline = "Не то. Попробуйте ещё раз";
    tone = "bad";
  } else if (idx > 1) {
    headline = "Верно! Продолжайте";
    tone = "ok";
  } else {
    headline = `Ваш ход. Найдите лучший ход за ${mySide === "white" ? "белых" : "чёрных"}`;
  }

  const delta = result?.counted ? result.after - result.before : null;

  return (
    <div className="room puzzles">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar puzzles__top">
            <span className="puzzles__ico">
              <Icon name="puzzles" size={22} />
            </span>
            <div className="pbar__who">
              <span className="pbar__name">Задача</span>
              <span className="pbar__sub">
                {finished ? `Рейтинг задачи: ${puzzle.rating}` : "Рейтинг задачи откроется после решения"}
              </span>
            </div>
            {settings.puzzleTimer && (
              <span className="puzzles__timer" title="Время на задачу">
                {Math.floor(elapsed / 60)}:{String(elapsed % 60).padStart(2, "0")}
              </span>
            )}
          </div>
          <div className="room__board">
            <Board
              state={boardState}
              orientation={viewSide}
              disabled={phase !== "solving"}
              lastMove={last}
              hint={hint}
              badge={badge}
              onPlayerMove={onPlayerMove}
            />
          </div>
          <div className="pbar">
            <Avatar user={user} size={36} />
            <div className="pbar__who">
              <span className="pbar__name">
                {myName}
                <span className="pbar__you">вы</span>
              </span>
              <span className="pbar__sub">
                {mySide === "white" ? "Белые" : "Чёрные"}
                <Captured fen={fen} side={mySide} leadOnly />
              </span>
            </div>
            {you && settings.puzzleRating && (
              <span className="puzzles__rating" title="Ваш рейтинг в задачах">
                {you.rating}
              </span>
            )}
          </div>
        </div>

        <aside className="room__side">
          <PuzzleTabs mode={mode} />

          {error && <div className="banner banner--error">{error}</div>}

          <section className={`card room__status puzzles__state${tone ? ` puzzles__state--${tone}` : ""}`} aria-live="polite">
            <p className="room__state">{headline}</p>
            {delta !== null && settings.puzzleRating && (
              <p className={`puzzles__delta puzzles__delta--${delta > 0 ? "up" : delta < 0 ? "down" : "same"}`}>
                {signed(delta)} <span>рейтинг {result.before} → {result.after}</span>
              </p>
            )}
            {result && !result.counted && <p className="room__meta">Эту задачу вы уже решали, рейтинг не изменился.</p>}
            {failed && !result && phase !== "shown" && mode === "rated" && <p className="room__meta">Рейтинг за эту задачу уже учтён.</p>}
            {finished && mode !== "rated" && (
              <p className="room__meta">Это тренировка: рейтинг не меняется. Он считается только в рейтинговых задачах.</p>
            )}
            {(finished || settings.puzzleThemes) && (
              <div className="puzzles__themes">
                {ideaThemes(puzzle.themes).map((t) => (
                  <span key={t} className="puzzles__chip">
                    {themeName(t)}
                  </span>
                ))}
              </div>
            )}
          </section>

          {mode === "daily" && daily && (
            <section className="card puzzles__daily">
              <h2>Задача дня</h2>
              <p className="puzzles__big puzzles__big--text">{dayLabel(daily.date)}</p>
              <p className="room__meta">
                {daily.status === "solved" && "Вы решили её с первой попытки."}
                {daily.status === "failed" && "Первая попытка была неудачной, рейтинг не изменится."}
                {daily.status === "new" && "Одна на всех, меняется в полночь по Гринвичу."}
              </p>
            </section>
          )}

          <section className="card room__actions">
            <div className="room__row">
              <button type="button" className="btn" onClick={showHint} disabled={phase !== "solving" || hint !== null}>
                <Icon name="star" size={16} /> Подсказка
              </button>
              <button type="button" className="btn" onClick={showSolution} disabled={phase !== "solving"}>
                <Icon name="check" size={16} /> Решение
              </button>
            </div>
            {mode === "daily" ? (
              finished && (
                <button type="button" className="btn btn--primary" onClick={() => chooseMode("rated")}>
                  К остальным задачам
                </button>
              )
            ) : (
              <button
                type="button"
                className={`btn ${finished ? "btn--primary" : "btn--ghost"}`}
                onClick={() => load(mode === "themes" ? { theme } : {})}
                disabled={loading}
              >
                {finished ? "Следующая задача" : "Пропустить"}
              </button>
            )}
          </section>

          {you && settings.puzzleRating && (
            <section className="card puzzles__you">
              <h2>Ваш рейтинг</h2>
              <p className="puzzles__big">{you.rating}</p>
              <p className="room__meta">
                Решено {you.solved} из {you.attempts}
                {user?.kind === "guest" ? ". Войдите через Google, чтобы рейтинг не потерялся." : ""}
              </p>
            </section>
          )}

          {mode === "themes" && (
            <section className="card puzzles__filter">
              <label htmlFor="puzzle-theme">Тема</label>
              <select id="puzzle-theme" className="field" value={theme} onChange={(e) => chooseTheme(e.target.value)} disabled={loading}>
                {themes.map((t) => (
                  <option key={t.id} value={t.id}>
                    {themeName(t.id)} ({t.count})
                  </option>
                ))}
              </select>
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}

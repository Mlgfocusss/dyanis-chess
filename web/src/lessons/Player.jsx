import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import BotAvatar from "../bots/BotAvatar.jsx";
import Board from "../components/Board.jsx";
import Icon from "../ui/Icon.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import { fetchLesson, saveProgress } from "./api.js";
import {
  PASS,
  boardArrows,
  highlightCss,
  hintSquare,
  judge,
  legalMoves,
  mainLine,
  minutesLabel,
  progressBody,
  resultSummary,
  squaresOf,
  startStep,
  stepCounter,
  wrongMessage,
} from "./lesson.js";

const DIONIS = { id: "dionis", name: "Дионис Самусенко" };
const REPLY_MS = 550; // pause before the opponent answers
const SHOW_MS = 800; // between the moves of a demonstration or of the shown solution
const ADVANCE_MS = 1500; // a solved exercise opens the next step by itself after this long

// The colour of a token (--primary and the rest) as the arrows of the board need it: a real colour.
function tokenColour(name) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(`--${name}`).trim();
  return v || "currentColor";
}

// A lesson: the board on the left, the step on the right. A text step explains with a position,
// arrows and highlighted squares, and may play a demonstration; an exercise asks the person to
// move on the board.
export default function Player({ id }) {
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();

  const [lesson, setLesson] = useState(null);
  const [error, setError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [idx, setIdx] = useState(0);
  const [finished, setFinished] = useState(false);
  const [mistakes, setMistakes] = useState(0);
  const mistakesRef = useRef(0);

  // the state of the step on the screen
  const [fen, setFen] = useState("");
  const [last, setLast] = useState(null);
  const [phase, setPhase] = useState("idle"); // text: idle | demo | demoDone; exercise: solving | reply | shown | solved
  const [node, setNode] = useState(null); // the position the person moves in
  const [depth, setDepth] = useState(0); // which move of the exercise it is
  const [feedback, setFeedback] = useState(null); // { kind: "ok" | "bad" | "info", text }
  const [hint, setHint] = useState(null); // { square, text }
  const [badge, setBadge] = useState(null);
  const [wrongHere, setWrongHere] = useState(0);
  const [auto, setAuto] = useState(false); // the next step is about to open by itself

  const timers = useRef([]);
  const later = useCallback((fn, ms) => {
    timers.current.push(setTimeout(fn, ms));
  }, []);
  const clearTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
  }, []);
  useEffect(() => clearTimers, [clearTimers]);

  const sound = useCallback(
    (san) => {
      if (settings.sound && san) playSound(soundKindForSan(san), settings.soundTheme);
    },
    [settings.sound, settings.soundTheme]
  );

  // Load the lesson (a guest account is made for a person who has none: it keeps the progress).
  useEffect(() => {
    if (status === "loading") return undefined;
    let alive = true;
    (async () => {
      try {
        if (status === "ready" && !user) await guest();
        const l = await fetchLesson(id);
        if (!alive) return;
        const from = startStep(l.progress, l.steps.length);
        mistakesRef.current = l.progress && !l.progress.completed && from > 0 ? l.progress.mistakes : 0;
        setMistakes(mistakesRef.current);
        setIdx(from);
        setLesson(l);
      } catch (e) {
        if (alive) setError(status === "unavailable" ? "Уроки недоступны: сервер запущен без базы данных." : e.message);
      }
    })();
    return () => {
      alive = false;
    };
  }, [status]); // eslint-disable-line react-hooks/exhaustive-deps

  const steps = lesson?.steps || [];
  const total = steps.length;
  const step = steps[idx];
  const isExercise = step?.type === "exercise";

  // A step begins: its position, and for an exercise the tree of its solutions.
  useEffect(() => {
    if (!lesson || finished) return;
    clearTimers();
    const s = lesson.steps[idx];
    setFen(s.fen);
    setLast(null);
    setBadge(null);
    setHint(null);
    setFeedback(null);
    setWrongHere(0);
    setAuto(false);
    setDepth(0);
    if (s.type === "exercise") {
      setNode(s.tree);
      setPhase("solving");
    } else {
      setNode(null);
      setPhase("idle");
    }
  }, [lesson, idx, finished, clearTimers]);

  // Saves how far the person has got; a failure is shown but does not stop the lesson.
  const commit = useCallback(
    (stepsDone) => {
      if (!lesson) return;
      saveProgress(lesson.id, progressBody(stepsDone, lesson.steps.length, mistakesRef.current)).then(
        () => setSaveError(""),
        (e) => setSaveError(`Прогресс не сохранился: ${e.message}`)
      );
    },
    [lesson]
  );

  function flash(square, cls, ms) {
    setBadge({ square, cls });
    later(() => setBadge(null), ms);
  }

  // Opens the step after `from`, or the end screen. It does not look at the phase: the timer that
  // calls it was set when the exercise was solved.
  function advance(from) {
    clearTimers();
    if (from === total - 1) setFinished(true);
    else setIdx(from + 1);
  }

  // The person solved it: after the last move has been seen, the next step opens by itself.
  function finishExercise() {
    setPhase("solved");
    setAuto(true);
    setFeedback({ kind: "ok", text: step.success });
    commit(idx + 1);
    later(() => advance(idx), ADVANCE_MS);
  }

  function onPlayerMove(uci) {
    if (phase !== "solving" || !isExercise) return { ok: false };
    const to = uci.slice(2, 4);
    const j = judge(node, uci);
    if (j.kind === "wrong") {
      const n = wrongHere + 1;
      setWrongHere(n);
      mistakesRef.current += 1;
      setMistakes(mistakesRef.current);
      flash(to, "blunder", 1200);
      setFeedback({ kind: "bad", text: wrongMessage(step, uci, depth, n) });
      return { ok: false };
    }
    const a = j.accept;
    setFen(a.fen);
    setLast(squaresOf(uci));
    setHint(null);
    sound(a.san);
    flash(to, "best", 900);
    if (!a.reply) {
      finishExercise();
      return { ok: true };
    }
    // the opponent answers
    setPhase("reply");
    setFeedback({ kind: "ok", text: "Верно!" });
    later(() => {
      setFen(a.reply.fen);
      setLast(squaresOf(a.reply.uci));
      sound(a.reply.san);
      if (!a.next) {
        finishExercise();
      } else {
        setNode(a.next);
        setDepth((d) => d + 1);
        setPhase("solving");
        setFeedback({ kind: "ok", text: "Верно! Продолжайте." });
      }
    }, REPLY_MS);
    return { ok: true };
  }

  function showHint() {
    if (phase !== "solving" || !node) return;
    setHint({ square: hintSquare(node), text: step.hint });
  }

  function showSolution() {
    if (phase !== "solving" || !node) return;
    clearTimers();
    setHint(null);
    setBadge(null);
    setPhase("shown");
    setFeedback({ kind: "info", text: "Смотрите, как решается." });
    const line = mainLine(node);
    line.forEach((m, i) => {
      later(() => {
        setFen(m.fen);
        setLast(squaresOf(m.uci));
        sound(m.san);
        if (i === line.length - 1) {
          setPhase("solved");
          setFeedback({ kind: "info", text: `Это решение. ${step.success}` });
          commit(idx + 1);
        }
      }, SHOW_MS * (i + 1));
    });
  }

  function playDemo() {
    if (isExercise || !step.demo?.length || phase === "demo") return;
    clearTimers();
    setPhase("demo");
    setFen(step.fen);
    setLast(null);
    let t = 0;
    step.demo.forEach((m, i) => {
      t += m.uci === PASS ? 1 : SHOW_MS;
      later(() => {
        setFen(m.fen);
        if (m.uci !== PASS) {
          setLast(squaresOf(m.uci));
          sound(m.san);
        }
        if (i === step.demo.length - 1) setPhase("demoDone");
      }, t);
    });
  }

  const canNext = !!step && (isExercise ? phase === "solved" : true);
  const isLast = idx === total - 1;

  function next() {
    if (!canNext) return;
    if (!isExercise) commit(idx + 1); // an exercise saved itself when it was solved
    advance(idx);
  }

  function prev() {
    if (idx === 0 || finished) return;
    clearTimers();
    setIdx(idx - 1);
  }

  function again() {
    mistakesRef.current = 0;
    setMistakes(0);
    setFinished(false);
    setIdx(0);
  }

  // Keys: ← and → move between steps, H asks for a hint, S shows the demonstration or the solution.
  const actions = useRef({});
  actions.current = { next, prev, showHint, showSolution, playDemo, canNext, isExercise, finished };
  useEffect(() => {
    const onKey = (e) => {
      if (e.ctrlKey || e.metaKey || e.altKey || e.target.closest?.("input, textarea, select")) return;
      const a = actions.current;
      if (a.finished) return;
      const key = e.key.toLowerCase();
      if (e.key === "ArrowRight" && a.canNext) {
        e.preventDefault();
        a.next();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        a.prev();
      } else if (key === "h" || key === "р") {
        a.showHint();
      } else if (key === "s" || key === "ы") {
        if (a.isExercise) a.showSolution();
        else a.playDemo();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // What the board shows. The arrows and highlighted squares of the step are for the starting
  // position; once a piece has moved they would point at the wrong thing.
  const atStart = !!step && fen === step.fen;
  const arrows = atStart && step ? boardArrows(step.arrows, tokenColour) : []; // read at each draw, so a change of theme recolours them
  const css = useMemo(() => (atStart && step ? highlightCss(step.highlights, ".lboard") : ""), [atStart, step]);

  if (error || !lesson || !step) {
    return (
      <div className="room lessons-player lessons-player--loading">
        {error ? (
          <div>
            <div className="banner banner--error">{error}</div>
            <a className="btn" href="#lessons">
              <Icon name="back" size={16} /> К списку уроков
            </a>
          </div>
        ) : (
          "Загружаю урок…"
        )}
      </div>
    );
  }

  const orientation = settings.whiteBottom ? "white" : step.orientation;
  const boardState = { fen, legalMoves: phase === "solving" ? legalMoves(node) : [] };
  const summary = resultSummary(mistakes);
  const nextLesson = lesson.next;

  return (
    <div className="lp">
      <div className="lp__grid">
        <div className="lp__boardwrap">
          <div className="room__board lboard">
            {css && <style>{css}</style>}
            <Board
              state={boardState}
              orientation={orientation}
              disabled={phase !== "solving" || finished}
              lastMove={last}
              hint={hint ? hint.square : null}
              badge={badge}
              arrows={arrows}
              onPlayerMove={onPlayerMove}
            />
          </div>
        </div>

        {/* the panel is as tall as the board; the big button is at its bottom, next to the board's lower edge */}
        <aside className="lp__panel card">
          <header className="lp__head">
            <a className="lp__back" href="#lessons" title="К списку уроков" aria-label="К списку уроков">
              <Icon name="back" size={20} />
            </a>
            <span className="lp__title">
              <Icon name="lessons" size={20} /> {lesson.title}
            </span>
            <span className="lp__mistakes" title="Ошибок в этом уроке" aria-label={`Ошибок: ${mistakes}`}>
              <Icon name="flag" size={14} /> {mistakes}
            </span>
          </header>

          {finished ? (
            <>
              <div className="lp__scroll lp__result" aria-live="polite">
                <span className="lp__trophy">
                  <Icon name="trophy" size={28} />
                </span>
                <h2 className="lp__h">{summary.title}</h2>
                <p className="lp__text">{summary.text}</p>
                <p className="lp__meta">
                  {lesson.title} · {minutesLabel(lesson.minutes)}
                </p>
                {!nextLesson && <p className="lp__meta">Это был последний урок. Загляните в раздел задач, чтобы закрепить знания.</p>}
                {saveError && <p className="room__notice">{saveError}</p>}
              </div>
              <div className="lp__bottom">
                {nextLesson ? (
                  <a className="btn btn--primary lp__next" href={`#lessons/${nextLesson.id}`}>
                    <span className="lp__next-label">
                      Следующий урок <Icon name="next" size={20} />
                    </span>
                  </a>
                ) : (
                  <a className="btn btn--primary lp__next" href="#lessons">
                    <span className="lp__next-label">К списку уроков</span>
                  </a>
                )}
                <div className="lp__tools">
                  {nextLesson && (
                    <a className="btn btn--ghost" href="#lessons">
                      К списку
                    </a>
                  )}
                  <button type="button" className="btn btn--ghost" onClick={again}>
                    Пройти ещё раз
                  </button>
                </div>
              </div>
            </>
          ) : (
            <>
              <div className="lp__scroll">
                <div className="lp__coach">
                  <BotAvatar bot={DIONIS} size={64} />
                  <div className="lp__bubble">
                    <b className="lp__kind">{isExercise ? "Упражнение" : step.title || "Объяснение"}</b>
                    {isExercise && step.title && <h2 className="lp__h">{step.title}</h2>}
                    <p className="lp__text">{step.text}</p>
                  </div>
                </div>

                <div aria-live="polite">
                  {feedback && <p className={`lp__fb lp__fb--${feedback.kind}`}>{feedback.text}</p>}
                  {hint && <p className="lp__fb lp__fb--info">{hint.text}</p>}
                </div>
                {saveError && <p className="room__notice">{saveError}</p>}
              </div>

              <div className="lp__prog">
                <span className="lp__count">{stepCounter(idx, total)}</span>
                <ol className="lp__strip" aria-label="Шаги урока">
                  {steps.map((s, i) => {
                    const state = i < idx ? "done" : i === idx ? "now" : "todo";
                    return (
                      <li key={i}>
                        <button
                          type="button"
                          className={`lp__seg lp__seg--${state}${s.type === "exercise" ? " lp__seg--ex" : ""}`}
                          disabled={i >= idx}
                          onClick={() => {
                            clearTimers();
                            setIdx(i);
                          }}
                          aria-label={`${stepCounter(i, total)}${s.type === "exercise" ? ", упражнение" : ""}`}
                          aria-current={i === idx ? "step" : undefined}
                        />
                      </li>
                    );
                  })}
                </ol>
              </div>

              <div className="lp__bottom">
                <button
                  type="button"
                  className={`btn btn--primary lp__next${auto ? " lp__next--auto" : ""}`}
                  onClick={next}
                  disabled={!canNext}
                >
                  <span className="lp__next-label">
                    {isLast ? "Завершить" : "Далее"} <Icon name="next" size={20} />
                  </span>
                </button>
                <div className="lp__tools">
                  {isExercise ? (
                    <>
                      <button type="button" className="btn btn--ghost" onClick={showHint} disabled={phase !== "solving" || !!hint} title="Подсказка (H)">
                        <Icon name="star" size={16} /> Подсказка
                      </button>
                      <button type="button" className="btn btn--ghost" onClick={showSolution} disabled={phase !== "solving"} title="Решение (S)">
                        <Icon name="check" size={16} /> Решение
                      </button>
                    </>
                  ) : (
                    step.demo?.length > 0 && (
                      <button type="button" className="btn btn--ghost" onClick={playDemo} disabled={phase === "demo"} title="Показать (S)">
                        <Icon name="play" size={16} /> {phase === "demoDone" ? "Повторить" : "Показать"}
                      </button>
                    )
                  )}
                  <button type="button" className="btn btn--ghost lp__tool-prev" onClick={prev} disabled={idx === 0} aria-label="Предыдущий шаг" title="Назад (←)">
                    <Icon name="prev" size={16} /> Назад
                  </button>
                </div>
              </div>
            </>
          )}
        </aside>
      </div>
    </div>
  );
}

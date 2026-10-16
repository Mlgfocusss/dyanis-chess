import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import Icon from "../ui/Icon.jsx";
import { fetchLine } from "../review/api.js";
import { sideOf } from "../review/branch.js";
import { fetchSession, sendReview } from "./api.js";
import { advance, deviationText, deviationVerdict, judge, reportFor, startQueue, summary } from "./session.js";
import { START_FEN, lineText, whenText } from "./tree.js";

const PATH_MS = 380; // between the moves of the line that is played before the question
const NEXT_MS = 750; // pause after a move before the next question
const NEW_PER_SESSION = 10;

const arrowOf = (uci) => [[uci.slice(0, 2), uci.slice(2, 4), "rgba(34, 197, 94, 0.9)"]];

// The trainer: the line is played up to a position, and the person plays
// their move. See session.js for how the queue and the reports to the server work.
export default function Trainer({ id }) {
  const { settings } = useSettings();
  const [session, setSession] = useState(null); // the server's plan
  const [error, setError] = useState("");
  const [queue, setQueue] = useState([]);
  const [phase, setPhase] = useState("loading"); // loading | path | show | ask | reveal | between | done | empty
  const [fen, setFen] = useState(START_FEN);
  const [last, setLast] = useState(null);
  const [message, setMessage] = useState("");
  const [messageKind, setMessageKind] = useState(""); // how a deviation was judged: fine | inaccurate | blunder
  const [hint, setHint] = useState(null);
  const [badge, setBadge] = useState(null);
  const [boardKey, setBoardKey] = useState(0);
  const [results, setResults] = useState({}); // move -> true | false, the first result of each

  const timers = useRef([]);
  const mistake = useRef(false);
  const reported = useRef(new Set());
  const explaining = useRef(0); // which question the engine's verdict on a deviation belongs to
  const clear = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
  }, []);
  const later = useCallback((fn, ms) => timers.current.push(setTimeout(fn, ms)), []);
  useEffect(() => clear, [clear]);

  const sound = useCallback(
    (san) => {
      if (settings.sound) playSound(soundKindForSan(san), settings.soundTheme);
    },
    [settings.sound, settings.soundTheme]
  );

  const entry = queue[0];
  const drill = entry?.drill;

  // Plays the line up to the question, then waits for the person's move.
  const begin = useCallback(
    (e) => {
      clear();
      mistake.current = false;
      explaining.current++;
      setHint(null);
      setBadge(null);
      setMessage("");
      setMessageKind("");
      setLast(null);
      setFen(START_FEN);
      setPhase("path");
      const d = e.drill;
      const finish = () => {
        setFen(d.fen);
        setPhase(e.show ? "show" : "ask");
      };
      if (!d.path.length) return finish();
      d.path.forEach((step, i) => {
        later(() => {
          setFen(step.fen);
          setLast({ from: step.uci.slice(0, 2), to: step.uci.slice(2, 4) });
          sound(step.san);
        }, PATH_MS * (i + 1));
      });
      later(finish, PATH_MS * (d.path.length + 1));
    },
    [clear, later, sound]
  );

  const start = useCallback(async () => {
    clear();
    setError("");
    setPhase("loading");
    setResults({});
    reported.current = new Set();
    try {
      const s = await fetchSession(id, NEW_PER_SESSION);
      setSession(s);
      const q = startQueue(s.drills);
      setQueue(q);
      if (q.length) begin(q[0]);
      else setPhase("empty");
    } catch (e) {
      setError(e.message);
    }
  }, [id, begin, clear]);
  useEffect(() => {
    start();
  }, [start]);

  function skipPath() {
    clear();
    setFen(drill.fen);
    setLast(drill.path.length ? { from: drill.path.at(-1).uci.slice(0, 2), to: drill.path.at(-1).uci.slice(2, 4) } : null);
    setPhase(entry.show ? "show" : "ask");
  }

  // The entry is done with: tell the server the first result of the move, then go on.
  function finish(outcome) {
    const report = reportFor(reported.current, entry, outcome);
    if (report !== null) {
      reported.current.add(drill.move);
      setResults((r) => ({ ...r, [drill.move]: report }));
      sendReview(id, drill.move, report).catch((e) => setError(e.message));
    }
    const next = advance(queue, outcome);
    setQueue(next);
    setPhase("between");
    later(() => (next.length ? begin(next[0]) : setPhase("done")), NEXT_MS);
  }

  function applyAnswer() {
    setFen(drill.answer.fen);
    setLast({ from: drill.answer.uci.slice(0, 2), to: drill.answer.uci.slice(2, 4) });
    sound(drill.answer.san);
    setHint(null);
  }

  function onPlayerMove(uci) {
    if (!drill) return { ok: false };
    if (phase === "show" || phase === "reveal") {
      if (uci !== drill.answer.uci) {
        setMessage(`Сыграйте ход ${drill.answer.san}: он показан стрелкой.`);
        return { ok: false };
      }
      applyAnswer();
      finish(phase === "show" ? "shown" : "wrong");
      return { ok: true };
    }
    if (phase !== "ask") return { ok: false };
    const verdict = judge(drill, uci);
    if (verdict === "right") {
      applyAnswer();
      setBadge({ square: uci.slice(2, 4), cls: mistake.current ? "good" : "best" });
      setMessage(mistake.current ? "Верно. Но этот ход вернётся ещё раз." : "Верно!");
      finish(mistake.current ? "wrong" : "right");
      return { ok: true };
    }
    if (verdict === "alt") {
      setMessageKind("fine");
      setMessage("Этот ход тоже есть в вашем репозитории, но сейчас мы учим другую ветку. Попробуйте ещё.");
      return { ok: false };
    }
    mistake.current = true;
    explain(uci);
    setPhase("reveal");
    setBoardKey((k) => k + 1);
    return { ok: false };
  }

  // The person played a move that is not the learned one: the engine says how good it is.
  // It is a good move in another branch, a small inaccuracy, or a gross mistake.
  function explain(uci) {
    const d = drill;
    const mine = ++explaining.current;
    setMessageKind("");
    setMessage("Неверно. Смотрю, насколько хорош ваш ход…");
    Promise.all([
      fetchLine({ fen: d.fen, moves: [d.answer.uci], eval: true, depth: 10 }),
      fetchLine({ fen: d.fen, moves: [uci], eval: true, depth: 10 }),
    ])
      .then(([answer, played]) => {
        if (explaining.current !== mine) return;
        const verdict = deviationVerdict(answer.eval, played.eval, sideOf(d.fen));
        setMessage(deviationText(verdict, played.steps[0].san, d.answer.san));
        setMessageKind(verdict.kind);
        const cls = { fine: "good", inaccurate: "inaccuracy", blunder: "blunder" }[verdict.kind];
        if (cls) {
          setBadge({ square: uci.slice(2, 4), cls });
          later(() => setBadge(null), 3000);
        }
      })
      .catch(() => {
        if (explaining.current === mine) setMessage(`Неверно. Мы учим другой ход: ${d.answer.san}. Он показан стрелкой.`);
      });
  }

  function giveHint() {
    mistake.current = true; // a hint is help: the move comes back
    setHint(drill.answer.uci.slice(0, 2));
    setMessage("Подсказка использована: ход вернётся позже.");
  }

  function giveUp() {
    mistake.current = true;
    setPhase("reveal");
    setMessage(`Правильный ход: ${drill.answer.san}. Сыграйте его.`);
  }

  const boardState = useMemo(
    () => ({ fen, legalMoves: drill && (phase === "show" || phase === "ask" || phase === "reveal") ? drill.legal.map((uci) => ({ uci })) : [] }),
    [fen, drill, phase]
  );

  if (error && !session) {
    return (
      <div className="page">
        <div className="banner banner--error">{error}</div>
        <a className="btn" href="#openings">
          К списку репозиториев
        </a>
      </div>
    );
  }
  if (!session) return <div className="room openings openings--loading">Готовлю тренировку…</div>;

  const color = session.color;
  const sum = summary(results);
  const interactive = phase === "show" || phase === "ask" || phase === "reveal";
  const remaining = queue.length;

  let headline = "";
  if (phase === "path") headline = "Играем линию…";
  else if (phase === "show") headline = `Новый ход: ${drill.answer.san}`;
  else if (phase === "ask") headline = "Ваш ход";
  else if (phase === "reveal") headline = `Правильный ход: ${drill.answer.san}`;

  return (
    <div className="room openings">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar">
            <span className={`repcard__color repcard__color--${color === "white" ? "black" : "white"}`} />
            <div className="pbar__who">
              <span className="pbar__name">Соперник</span>
              <span className="pbar__sub">{color === "white" ? "Чёрные" : "Белые"}</span>
            </div>
          </div>
          <div className="room__board">
            <Board
              key={boardKey}
              state={boardState}
              orientation={color}
              disabled={!interactive}
              lastMove={last}
              hint={hint}
              badge={badge}
              arrows={drill && (phase === "show" || phase === "reveal") ? arrowOf(drill.answer.uci) : []}
              onPlayerMove={onPlayerMove}
            />
          </div>
          <div className="pbar">
            <span className={`repcard__color repcard__color--${color}`} />
            <div className="pbar__who">
              <span className="pbar__name">Вы</span>
              <span className="pbar__sub">{color === "white" ? "Белые" : "Чёрные"}</span>
            </div>
          </div>
        </div>

        <aside className="room__side">
          {error && <div className="banner banner--error">{error}</div>}

          <section className="card openings__head">
            <a className="openings__back" href={`#openings/${id}`}>
              <Icon name="back" size={16} /> К репозиторию «{session.name}»
            </a>
            {phase !== "done" && phase !== "empty" && (
              <>
                <p className="room__meta">
                  Осталось вопросов: {remaining} (по одному на ход). Верный ответ с первого раза выучивает ход, ошибка возвращает его позже.
                </p>
                <div className="repbar">
                  <div className="repbar__fill" style={{ width: `${session.drills.length ? (100 * (session.drills.length - Math.min(remaining, session.drills.length))) / session.drills.length : 0}%` }} />
                </div>
              </>
            )}
          </section>

          {(phase === "path" || phase === "show" || phase === "ask" || phase === "reveal" || phase === "between") && (
            <section className="card openings__pos">
              <p className="room__state">{phase === "between" ? message || "Дальше…" : headline}</p>
              {phase === "path" && (
                <>
                  <p className="room__meta">Соперник и вы играете линию репозитория до нужной позиции.</p>
                  <button type="button" className="btn" onClick={skipPath}>
                    Пропустить показ
                  </button>
                </>
              )}
              {phase === "show" && <p className="room__meta">Это новый ход репозитория. Сыграйте его на доске, чтобы запомнить: потом он вернётся как вопрос.</p>}
              {phase === "ask" && (
                <>
                  <p className="room__meta">Какой ход вы играете в этой позиции по своему репозиторию?</p>
                  <div className="room__row">
                    <button type="button" className="btn" onClick={giveHint} disabled={hint !== null}>
                      <Icon name="star" size={16} /> Подсказка
                    </button>
                    <button type="button" className="btn" onClick={giveUp}>
                      Показать ход
                    </button>
                  </div>
                </>
              )}
              {phase !== "between" && message && (
                <p className={`openings__msg${message.startsWith("Верно") ? " openings__msg--ok" : messageKind ? ` openings__msg--${messageKind}` : ""}`}>{message}</p>
              )}
              {drill && drill.path.length > 0 && phase !== "path" && <p className="openings__line">{lineText(drill.path)}</p>}
            </section>
          )}

          {phase === "empty" && (
            <section className="card openings__pos">
              <p className="room__state">На сегодня всё</p>
              <p className="room__meta">
                {session.nextDue ? `Ближайшее повторение: ${whenText(session.nextDue)}.` : "В репозитории нет ваших ходов: добавьте их в конструкторе."}
              </p>
              <a className="btn btn--primary" href={`#openings/${id}`}>
                К репозиторию
              </a>
            </section>
          )}

          {phase === "done" && (
            <section className="card openings__pos">
              <p className="room__state">Тренировка окончена</p>
              <p className="room__meta">
                Ходов проверено: {sum.total}. Верно с первого раза: {sum.right}. С ошибкой: {sum.wrong}.
              </p>
              {sum.wrong > 0 && <p className="room__meta">Ошибочные ходы вернутся скоро, а остальные через сутки и дальше всё реже.</p>}
              <div className="room__row">
                <button type="button" className="btn btn--primary" onClick={start}>
                  Ещё порция
                </button>
                <a className="btn" href={`#openings/${id}`}>
                  К репозиторию
                </a>
              </div>
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}

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
import { useT } from "../i18n/index.js";

const PATH_MS = 380; // between the moves of the line that is played before the question
const NEXT_MS = 750; // pause after a move before the next question
const NEW_PER_SESSION = 10;

const arrowOf = (uci) => [[uci.slice(0, 2), uci.slice(2, 4), "rgba(34, 197, 94, 0.9)"]];

// The trainer: the line is played up to a position, and the person plays
// their move. See session.js for how the queue and the reports to the server work.
export default function Trainer({ id }) {
  const t = useT();
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
        setMessage(t("trainer.1", { san: drill.answer.san }));
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
      setMessage(mistake.current ? t("trainer.2") : t("trainer.3"));
      finish(mistake.current ? "wrong" : "right");
      return { ok: true };
    }
    if (verdict === "alt") {
      setMessageKind("fine");
      setMessage(t("trainer.4"));
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
    setMessage(t("trainer.5"));
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
        if (explaining.current === mine) setMessage(t("trainer.6", { san: d.answer.san }));
      });
  }

  function giveHint() {
    mistake.current = true; // a hint is help: the move comes back
    setHint(drill.answer.uci.slice(0, 2));
    setMessage(t("trainer.7"));
  }

  function giveUp() {
    mistake.current = true;
    setPhase("reveal");
    setMessage(t("trainer.8", { san: drill.answer.san }));
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
          {t("builder.9")}
        </a>
      </div>
    );
  }
  if (!session) return <div className="room openings openings--loading">{t("trainer.10")}</div>;

  const color = session.color;
  const sum = summary(results);
  const interactive = phase === "show" || phase === "ask" || phase === "reveal";
  const remaining = queue.length;

  let headline = "";
  if (phase === "path") headline = t("trainer.11");
  else if (phase === "show") headline = t("trainer.12", { san: drill.answer.san });
  else if (phase === "ask") headline = t("gameRoom.41");
  else if (phase === "reveal") headline = t("trainer.14", { san: drill.answer.san });

  return (
    <div className="room openings">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar">
            <span className={`repcard__color repcard__color--${color === "white" ? "black" : "white"}`} />
            <div className="pbar__who">
              <span className="pbar__name">{t("openings.opponent")}</span>
              <span className="pbar__sub">{color === "white" ? t("online.black") : t("online.white")}</span>
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
              <span className="pbar__name">{t("botsPage.8")}</span>
              <span className="pbar__sub">{color === "white" ? t("online.white") : t("online.black")}</span>
            </div>
          </div>
        </div>

        <aside className="room__side">
          {error && <div className="banner banner--error">{error}</div>}

          <section className="card openings__head">
            <a className="openings__back" href={`#openings/${id}`}>
              <Icon name="back" size={16} /> {t("trainer.19")}{session.name}»
            </a>
            {phase !== "done" && phase !== "empty" && (
              <>
                <p className="room__meta">
                  {t("trainer.20")} {remaining} {t("trainer.21")}
                </p>
                <div className="repbar">
                  <div className="repbar__fill" style={{ width: `${session.drills.length ? (100 * (session.drills.length - Math.min(remaining, session.drills.length))) / session.drills.length : 0}%` }} />
                </div>
              </>
            )}
          </section>

          {(phase === "path" || phase === "show" || phase === "ask" || phase === "reveal" || phase === "between") && (
            <section className="card openings__pos">
              <p className="room__state">{phase === "between" ? message || t("trainer.next") : headline}</p>
              {phase === "path" && (
                <>
                  <p className="room__meta">{t("trainer.23")}</p>
                  <button type="button" className="btn" onClick={skipPath}>
                    {t("trainer.24")}
                  </button>
                </>
              )}
              {phase === "show" && <p className="room__meta">{t("trainer.25")}</p>}
              {phase === "ask" && (
                <>
                  <p className="room__meta">{t("trainer.26")}</p>
                  <div className="room__row">
                    <button type="button" className="btn" onClick={giveHint} disabled={hint !== null}>
                      <Icon name="star" size={16} /> {t("trainer.27")}
                    </button>
                    <button type="button" className="btn" onClick={giveUp}>
                      {t("trainer.28")}
                    </button>
                  </div>
                </>
              )}
              {phase !== "between" && message && (
                <p className={`openings__msg${message.startsWith(t("trainer.29")) ? " openings__msg--ok" : messageKind ? ` openings__msg--${messageKind}` : ""}`}>{message}</p>
              )}
              {drill && drill.path.length > 0 && phase !== "path" && <p className="openings__line">{lineText(drill.path)}</p>}
            </section>
          )}

          {phase === "empty" && (
            <section className="card openings__pos">
              <p className="room__state">{t("trainer.30")}</p>
              <p className="room__meta">
                {session.nextDue ? t("trainer.31", { p: whenText(session.nextDue) }) : t("trainer.32")}
              </p>
              <a className="btn btn--primary" href={`#openings/${id}`}>
                {t("trainer.33")}
              </a>
            </section>
          )}

          {phase === "done" && (
            <section className="card openings__pos">
              <p className="room__state">{t("trainer.34")}</p>
              <p className="room__meta">
                {t("trainer.35")} {sum.total}{t("trainer.36")} {sum.right}{t("trainer.37")} {sum.wrong}.
              </p>
              {sum.wrong > 0 && <p className="room__meta">{t("trainer.38")}</p>}
              <div className="room__row">
                <button type="button" className="btn btn--primary" onClick={start}>
                  {t("trainer.39")}
                </button>
                <a className="btn" href={`#openings/${id}`}>
                  {t("trainer.33")}
                </a>
              </div>
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Avatar from "../auth/Avatar.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import { changeLine, ratingText } from "../online/rating.js";
import { START_FEN } from "../play/game.js";
import Icon from "../ui/Icon.jsx";
import PuzzleTabs from "./PuzzleTabs.jsx";
import { fetchDuel, fetchDuelPuzzle, sendDuelProgress } from "./api.js";
import { judgeMove, legalFor, squaresOf } from "./puzzle.js";
import { clockText } from "./rush.js";
import "./puzzles.css";

const POLL_MS = 1000;
const INTRO_MS = 300;
const REPLY_MS = 250;
const FAIL_MS = 700; // how long a mistake is shown before the next puzzle

const OUTCOME = {
  win: { title: "Победа!", tone: "ok" },
  loss: { title: "Поражение", tone: "bad" },
  draw: { title: "Ничья", tone: "warn" },
};

// A puzzle duel in progress: the same puzzles for both people, a clock the server keeps, and the
// other person's score shown live.
export default function DuelRoom({ id }) {
  const { user } = useAuth();
  const { settings } = useSettings();

  const [duel, setDuel] = useState(null);
  const [error, setError] = useState("");
  const [tick, setTick] = useState(Date.now());
  const [puzzle, setPuzzle] = useState(null);
  const [fen, setFen] = useState(START_FEN);
  const [idx, setIdx] = useState(0);
  const [last, setLast] = useState(null);
  const [badge, setBadge] = useState(null);
  const [phase, setPhase] = useState("wait"); // wait | intro | solving | reply | between | over

  const offset = useRef(0); // server time minus ours, to read the server's clock
  const phaseRef = useRef("wait");
  const kRef = useRef(0); // the puzzle being solved
  const started = useRef(false);
  const cache = useRef(new Map()); // puzzle number -> promise of the puzzle
  const timers = useRef([]);
  const alive = useRef(true);
  const overRef = useRef(false);

  const setPh = useCallback((p) => {
    phaseRef.current = p;
    setPhase(p);
  }, []);
  const later = useCallback((fn, ms) => {
    timers.current.push(setTimeout(fn, ms));
  }, []);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      timers.current.forEach(clearTimeout);
    };
  }, []);

  const sound = useCallback(
    (kind) => {
      if (settings.sound) playSound(kind, settings.soundTheme);
    },
    [settings.sound, settings.soundTheme]
  );

  const accept = useCallback((d) => {
    offset.current = d.serverNow - Date.now();
    overRef.current = d.status === "over";
    setDuel(d);
    if (d.status === "over") {
      timers.current.forEach(clearTimeout);
      setPh("over");
    }
  }, [setPh]);

  // The state of the duel, every second until it is over and the result is in.
  useEffect(() => {
    let stop = false;
    let t;
    async function poll() {
      try {
        const d = await fetchDuel(id);
        if (stop) return;
        accept(d);
        if (d.status === "over" && d.outcome) return;
      } catch (e) {
        if (stop) return;
        if (e.status === 404 || e.status === 401) {
          setError("Такой дуэли нет или она закончилась давно.");
          return;
        }
      }
      t = setTimeout(poll, POLL_MS);
    }
    poll();
    return () => {
      stop = true;
      clearTimeout(t);
    };
  }, [id, accept]);

  useEffect(() => {
    const t = setInterval(() => setTick(Date.now()), 200);
    return () => clearInterval(t);
  }, []);

  const getPuzzle = useCallback(
    (k) => {
      if (!cache.current.has(k)) {
        cache.current.set(
          k,
          fetchDuelPuzzle(id, k).then(
            (d) => d.puzzle,
            (e) => {
              cache.current.delete(k);
              throw e;
            }
          )
        );
      }
      return cache.current.get(k);
    },
    [id]
  );

  // Puts puzzle number k on the board: the opponent's move that makes it, then the person solves it.
  const open = useCallback(
    async (k) => {
      try {
        const p = await getPuzzle(k);
        if (!alive.current || overRef.current) return;
        kRef.current = k;
        setPuzzle(p);
        setFen(p.fen);
        setIdx(0);
        setLast(null);
        setBadge(null);
        setPh("intro");
        getPuzzle(k + 1).catch(() => {}); // the next one is asked for while this is solved
        later(() => {
          if (overRef.current) return;
          const first = p.steps[0];
          setFen(first.fen);
          setLast(squaresOf(first.uci));
          sound(soundKindForSan(first.san));
          setIdx(1);
          setPh("solving");
        }, INTRO_MS);
      } catch (e) {
        if (alive.current) setError(e.message);
      }
    },
    [getPuzzle, later, setPh, sound]
  );

  // The duel has begun: start on the puzzle the server says this person is on (a reload mid-duel resumes).
  useEffect(() => {
    if (duel?.status === "running" && !started.current) {
      started.current = true;
      open(duel.me.index);
    }
  }, [duel, open]);

  // Tells the server how the puzzle ended and goes on to the next.
  async function report(solved) {
    const k = kRef.current;
    setPh("between");
    try {
      const d = await sendDuelProgress(id, k, solved);
      if (!alive.current) return;
      accept(d);
      open(k + 1);
    } catch (e) {
      if (!alive.current) return;
      if (e.status === 409) {
        try {
          const d = await fetchDuel(id); // out of step, or the time is up: ask where we are
          accept(d);
          if (d.status === "running") open(d.me.index);
        } catch {
          // the poll will tell
        }
        return;
      }
      setError(e.message);
    }
  }

  function onPlayerMove(uci) {
    if (phaseRef.current !== "solving" || overRef.current) return { ok: false };
    const j = judgeMove(puzzle, idx, uci);
    if (j.kind === "wrong") {
      setBadge({ square: uci.slice(2, 4), cls: "blunder" });
      setPh("between");
      later(() => report(false), FAIL_MS);
      return { ok: false };
    }
    setFen(j.fen);
    setLast(squaresOf(uci));
    sound(soundKindForSan(j.san));
    if (j.done) {
      report(true);
      return { ok: true };
    }
    const reply = puzzle.steps[idx + 1];
    setIdx(idx + 1);
    setPh("reply");
    later(() => {
      if (overRef.current) return;
      setFen(reply.fen);
      setLast(squaresOf(reply.uci));
      sound(soundKindForSan(reply.san));
      if (idx + 2 >= puzzle.steps.length) report(true); // the line ends with the opponent's move
      else {
        setIdx(idx + 2);
        setPh("solving");
      }
    }, REPLY_MS);
    return { ok: true };
  }

  if (error && !duel) {
    return (
      <div className="page lobby">
        <div className="banner banner--error">{error}</div>
        <a className="btn btn--primary" href="#puzzles/duel">
          К дуэлям
        </a>
      </div>
    );
  }
  if (!duel) return <div className="room puzzles puzzles--loading">Загружаю дуэль…</div>;

  const serverNow = tick + offset.current;
  const toStart = duel.startsAt - serverNow;
  const left = duel.endsAt - serverNow;
  const over = duel.status === "over";
  const running = duel.status === "running" && left > 0;
  const me = duel.me;
  const opp = duel.opponent;
  const mySide = settings.whiteBottom ? "white" : puzzle?.orientation || "white";
  const res = OUTCOME[duel.outcome];

  let clock;
  if (duel.status === "countdown") clock = `${Math.max(1, Math.ceil(toStart / 1000))}`;
  else clock = clockText(over ? 0 : left);

  return (
    <div className="room puzzles duelroom">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar puzzles__top">
            <Avatar user={{ name: opp.name, avatarUrl: opp.avatar }} size={36} />
            <div className="pbar__who">
              <span className="pbar__name">
                {opp.name}
                <span className="pbar__elo"> ({ratingText(opp.rating, opp.provisional)})</span>
              </span>
              <span className="pbar__sub">Соперник · задача {opp.index + 1}</span>
            </div>
            <span className="duelroom__score duelroom__score--opp" aria-label="Очки соперника">
              {opp.score}
            </span>
          </div>
          <div className="room__board">
            <Board
              state={{ fen, legalMoves: phase === "solving" && running ? legalFor(puzzle, idx) : [] }}
              orientation={mySide}
              disabled={!(phase === "solving" && running)}
              lastMove={last}
              badge={badge}
              onPlayerMove={onPlayerMove}
            />
            {duel.status === "countdown" && (
              <div className="duelroom__cover" aria-live="assertive">
                <span className="duelroom__count">{clock}</span>
                <span>Дуэль начнётся через несколько секунд</span>
              </div>
            )}
          </div>
          <div className="pbar">
            <Avatar user={user} size={36} />
            <div className="pbar__who">
              <span className="pbar__name">
                {user?.name || "Вы"}
                <span className="pbar__you">вы</span>
              </span>
              <span className="pbar__sub">
                {me.rating ? `Рейтинг ${ratingText(me.rating, me.provisional)} · ` : ""}ошибок {me.mistakes}
              </span>
            </div>
            <span className="duelroom__score" aria-label="Ваши очки">
              {me.score}
            </span>
          </div>
        </div>

        <aside className="room__side">
          <PuzzleTabs mode="duel" />
          {error && <div className="banner banner--error">{error}</div>}

          <section className={`card room__status puzzles__state${res ? ` puzzles__state--${res.tone}` : ""}`} aria-live="polite">
            <div className="duelroom__timer">
              <Icon name="clock" size={20} />
              <span className={left < 15000 && running ? "rush__clock--low" : ""}>{duel.status === "countdown" ? `Старт: ${clock}` : clockText(over ? 0 : left)}</span>
            </div>
            {!over && <p className="room__state">{duel.status === "countdown" ? "Приготовьтесь" : "Решайте быстрее соперника"}</p>}
            {over && !res && <p className="room__state">Подводим итоги…</p>}
            {over && res && (
              <>
                <p className="room__state">{res.title}</p>
                <p className="room__meta">
                  Счёт {me.score} : {opp.score}, ваших ошибок {me.mistakes}, у соперника {opp.mistakes}.
                </p>
                {me.change && <p className="room__meta">{changeLine(me.change)}</p>}
              </>
            )}
          </section>

          {over && (
            <section className="card room__actions">
              <a className="btn btn--primary" href="#puzzles/duel">
                <Icon name="duel" size={16} /> Новая дуэль
              </a>
            </section>
          )}
          {!over && (
            <section className="card puzzles__you">
              <h2>Счёт</h2>
              <p className="puzzles__big">
                {me.score} : {opp.score}
              </p>
              <p className="room__meta">Задачи у вас и соперника одни и те же, но темп у каждого свой.</p>
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}

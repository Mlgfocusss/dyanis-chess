import { useCallback, useEffect, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Avatar from "../auth/Avatar.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import { changeLine, ratingText } from "../online/rating.js";
import { START_FEN } from "../play/game.js";
import Icon from "../ui/Icon.jsx";
import { BigButton, Placeholder, Say, Shell } from "./PuzzleShell.jsx";
import { fetchDuel, fetchDuelPuzzle, sendDuelProgress } from "./api.js";
import { judgeMove, legalFor, squaresOf } from "./puzzle.js";
import { clockText } from "./rush.js";
import { useT } from "../i18n/index.js";

const POLL_MS = 1000;
const INTRO_MS = 300;
const REPLY_MS = 250;
const FAIL_MS = 700; // how long a mistake is shown before the next puzzle

const OUTCOME = {
  win: { title: "pz.duelWin", tone: "ok" },
  loss: { title: "onlineGames.4", tone: "bad" },
  draw: { title: "onlineGames.2", tone: "warn" },
};

// A puzzle duel in progress: the same puzzles for both people, a clock the server keeps, and the
// other person's score shown live.
export default function DuelRoom({ id }) {
  const t = useT();
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
          setError(t("duelRoom.4"));
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
          {t("duelRoom.5")}
        </a>
      </div>
    );
  }
  if (!duel) return <Placeholder>{t("duelRoom.6")}</Placeholder>;

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

  let say;
  if (over) {
    say = res
      ? { kind: t(res.title), tone: res.tone, text: t("duelRoom.7", { score: me.score, score2: opp.score, mistakes: me.mistakes, mistakes2: opp.mistakes, p: me.change ? ` ${changeLine(me.change)}` : "" }) }
      : { kind: t("duelRoom.8"), tone: "", text: "" };
  } else if (duel.status === "countdown") {
    say = { kind: t("duelRoom.9"), tone: "", text: t("duelRoom.10") };
  } else {
    say = { kind: t("duelRoom.11"), tone: "", text: t("duelRoom.12") };
  }

  return (
    <Shell
      className="pz--duel"
      mode="duel"
      icon="duel"
      title={t("menu.duel")}
      head={
        <span className={`pz__clock pz__clock--big${left < 15000 && running ? " pz__clock--low" : ""}`} aria-label={t("duelRoom.14")}>
          {duel.status === "countdown" ? t("duelRoom.15", { clock }) : clockText(over ? 0 : left)}
        </span>
      }
      board={
        <>
          <Board
            state={{ fen, legalMoves: phase === "solving" && running ? legalFor(puzzle, idx) : [] }}
            orientation={mySide}
            disabled={!(phase === "solving" && running)}
            lastMove={last}
            badge={badge}
            onPlayerMove={onPlayerMove}
          />
          {duel.status === "countdown" && (
            <div className="pz__count" aria-live="assertive">
              <span className="pz__count-num">{clock}</span>
              <span>{t("duelRoom.16")}</span>
            </div>
          )}
        </>
      }
      bottom={
        over && (
          <BigButton href="#puzzles/duel">
            <Icon name="duel" size={20} /> {t("duelRoom.17")}
          </BigButton>
        )
      }
    >
      {error && <div className="banner banner--error">{error}</div>}

      <div className="pz__duel">
        <div className="pz__who pz__who--me">
          <Avatar user={user} size={36} />
          <span className="pz__who-name">{user?.name || t("botsPage.8")}</span>
          <span className="pz__who-sub">
            {me.rating ? `${ratingText(me.rating, me.provisional)} · ` : ""}{t("duelRoom.19")} {me.mistakes}
          </span>
          <span className="pz__who-score" aria-label={t("duelRoom.20")}>
            {me.score}
          </span>
        </div>
        <span className="pz__colon">:</span>
        <div className="pz__who">
          <Avatar user={{ name: opp.name, avatarUrl: opp.avatar }} size={36} />
          <span className="pz__who-name">{opp.name}</span>
          <span className="pz__who-sub">
            {ratingText(opp.rating, opp.provisional)} {t("duelRoom.21")} {opp.index + 1}
          </span>
          <span className="pz__who-score" aria-label={t("duelRoom.22")}>
            {opp.score}
          </span>
        </div>
      </div>

      <Say kind={say.kind} tone={say.tone}>
        {say.text}
      </Say>
    </Shell>
  );
}

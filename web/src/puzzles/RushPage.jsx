import { useCallback, useEffect, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { START_FEN } from "../play/game.js";
import { BigButton, Placeholder, Say, Shell } from "./PuzzleShell.jsx";
import { fetchRushBest, fetchRushPuzzle, sendRushResult } from "./api.js";
import { judgeMove, legalFor, squaresOf } from "./puzzle.js";
import { colourLetter, premoveToPlay } from "../components/boardrules.js";
import { RUSH_MODES, clockText, runSummary, targetRating } from "./rush.js";
import { tx, useT } from "../i18n/index.js";

const MODE_KEY = "dyanis.rushMode";
const INTRO_MS = 350; // pause before the opponent's first move
const REPLY_MS = 300; // pause before the opponent answers
const NEXT_MS = 250; // pause between a solved puzzle and the next
const TICK_MS = 200;
const MAX_PREMOVES = 30; // how many premoves may wait

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
  const t = useT();
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
  const [pre, setPre] = useState([]); // premoves (uci) chosen while the opponent answers, played one by one on the person's turns

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
    setPre([]);
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
    setPre([]);
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

  // The first premove is played the moment it is the person's turn, if it is legal then; one that is not drops
  // the whole queue. A wrong move that is legal ends the run, as any wrong move does.
  useEffect(() => {
    if (!pre.length) return;
    if (phase === "between" || phase === "over" || phase === "idle") {
      setPre([]);
      return;
    }
    if (phase !== "solving") return;
    const play = premoveToPlay(legalFor(puzzle, idx), pre[0]);
    if (play) setPre(onPlayerMove(play).ok ? pre.slice(1) : []);
    else setPre([]);
  }, [phase, idx]); // eslint-disable-line react-hooks/exhaustive-deps

  if (status === "unavailable") {
    return (
      <Placeholder>
        <div className="banner banner--error">{t("puzzlesPage.4")}</div>
      </Placeholder>
    );
  }

  const running = phase !== "idle" && phase !== "over";
  const mySide = settings.whiteBottom ? "white" : puzzle?.orientation || "white";
  const modeBest = best[mode]?.best || 0;
  const low = running && left < 10_000;

  let say;
  if (over) {
    say = {
      kind: over.headline,
      tone: over.record ? "ok" : "warn",
      text:
        (over.record ? t("rushPage.2", { label: tx(RUSH_MODES[mode].label) }) : "") +
        (over.reason === "mistake" && over.expected ? t("rushPage.3", { expected: over.expected }) : ""),
    };
  } else if (running) {
    say =
      phase === "starting"
        ? { kind: t("rushPage.4"), tone: "", text: "" }
        : { kind: phase === "between" ? t("puzzleJs.12") : t("gameRoom.41"), tone: phase === "between" ? "ok" : "", text: t("rushPage.7") };
  } else {
    say = {
      kind: t("rushPage.8"),
      tone: "",
      text: t("rushPage.9"),
    };
  }

  return (
    <Shell
      className="pz--rush"
      mode="rush"
      icon="bolt"
      title={t("menu.rush")}
      head={
        <span className={`pz__clock pz__clock--big${low ? " pz__clock--low" : ""}`} aria-label={t("rushPage.11")}>
          {clockText(left)}
        </span>
      }
      board={
        <Board
          state={{ fen, legalMoves: phase === "solving" ? legalFor(puzzle, idx) : [] }}
          orientation={mySide}
          disabled={phase !== "solving"}
          lastMove={last}
          badge={badge}
          onPlayerMove={onPlayerMove}
          premove={{
            enabled: settings.premoveOn && phase === "reply",
            side: colourLetter(puzzle?.orientation || "white"),
            queue: pre,
            onSet: (uci) => setPre((q) => [...q, uci].slice(0, MAX_PREMOVES)),
            onClear: () => setPre([]),
          }}
        />
      }
      bottom={
        <>
          <BigButton onClick={start} disabled={running || status !== "ready"}>
            <Icon name="bolt" size={20} /> {running ? t("rushPage.12") : over ? t("rushPage.13") : t("rushPage.14")}
          </BigButton>
          <div className="pz__tools">
            <button type="button" className="btn btn--ghost" onClick={() => finish("quit")} disabled={!running}>
              {t("rushPage.15")}
            </button>
          </div>
        </>
      }
    >
      {error && <div className="banner banner--error">{error}</div>}

      <div className="pz__score">
        <span className="pz__label">{running || over ? t("rushPage.16") : t("rushPage.17", { label: tx(RUSH_MODES[mode].label) })}</span>
        <span className="pz__num">{running || over ? score : modeBest}</span>
        <span className="pz__sub">
          {t("rushPage.18")} {tx(RUSH_MODES[mode].label)}: <b>{modeBest}</b>{t("rushPage.19")} {best[mode]?.runs || 0}.
        </span>
      </div>

      <Say kind={say.kind} tone={say.tone}>
        {say.text}
      </Say>

      <div className="pz__field">
        <span className="pz__label">{t("rushPage.20")}</span>
        <Segmented label={t("rushPage.20")} value={mode} onChange={chooseMode} options={Object.entries(RUSH_MODES).map(([id, m]) => [id, tx(m.label)])} />
      </div>

      {user?.kind === "guest" && <p className="pz__meta">{t("rushPage.21")}</p>}
    </Shell>
  );
}

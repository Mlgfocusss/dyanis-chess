import { useCallback, useEffect, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import Icon from "../ui/Icon.jsx";
import { recordPractice } from "../trainer/trainer.js";
import { fetchPuzzle, fetchThemes, sendDailyResult, sendResult } from "./api.js";
import { coachSay, dayLabel, hintSquare, judgeMove, keyAction, legalFor, scoreView, squaresOf } from "./puzzle.js";
import { ideaThemes, themeName } from "./themes.js";
import { chooseMode } from "./PuzzleTabs.jsx";
import { BigButton, NextLabel, Placeholder, Say, Score, Shell } from "./PuzzleShell.jsx";
import PlyNav from "./PlyNav.jsx";
import RecentList from "./RecentList.jsx";
import { addRecent, readRecent, saveRecent } from "./recent.js";
import { encodeFen } from "../review/moments.js";
import { colourLetter, premoveToPlay } from "../components/boardrules.js";
import { useT } from "../i18n/index.js";

const THEME_KEY = "dyanis.puzzleTheme";
const INTRO_MS = 800; // pause before the opponent's first move
const REPLY_MS = 500; // pause before the opponent answers
const SHOW_MS = 800; // between the moves of the shown solution
const AUTO_MS = 1000; // a puzzle solved at the first try opens the next by itself after this long (if the setting is on)
const MAX_PREMOVES = 30; // how many premoves may wait

const TITLES = { rated: "pz.titleRated", themes: "pz.titleThemes", daily: "menu.puzzlesDaily" };

function savedTheme() {
  try {
    return localStorage.getItem(THEME_KEY) || "";
  } catch {
    return "";
  }
}

// "#puzzles" is the rated puzzles, "#puzzles/themes" and "#puzzles/daily" the other two tabs,
// "#puzzles/<id>" one particular puzzle (in the first tab), "#puzzles/<id>/view" the same to look at: solved, all moves shown.
function fromHash() {
  const [page, part, third] = window.location.hash.slice(1).split("/");
  if (page !== "puzzles" || !part || part === "rush" || part === "duel") return { mode: "rated", id: "" }; // those two are other pages (RushPage)
  if (part === "themes" || part === "daily") return { mode: part, id: "" };
  return { mode: "rated", id: part, view: third === "view" };
}

export default function PuzzlesPage() {
  const t = useT();
  const { status, user, guest } = useAuth();
  const { settings } = useSettings();

  const [mode, setMode] = useState(() => fromHash().mode); // rated | themes | daily
  // The rated puzzles always count the rating and the points together (the server does both in one
  // step). The setting "Собирать очки" only makes the points the main number and hides the rating.
  const pointsMain = settings.puzzlePoints && mode === "rated";
  const [theme, setTheme] = useState(savedTheme);
  const [themes, setThemes] = useState([]);
  const [puzzle, setPuzzle] = useState(null);
  const [daily, setDaily] = useState(null); // { date, status } while the puzzle of the day is on the board
  const [you, setYou] = useState(null); // { rating, attempts, solved }
  const [pts, setPts] = useState(null); // { total, streak, best, today, gained }
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
  const [auto, setAuto] = useState(false); // the next puzzle is about to open by itself
  const [trail, setTrail] = useState([]); // the positions shown so far: [{ fen, last, san }], the first is the start
  const [back, setBack] = useState(null); // the position of the trail being looked at, null = the current one
  const [viewOnly, setViewOnly] = useState(false); // a puzzle opened from the list to look at: no try, all moves shown
  const [recent, setRecent] = useState(() => readRecent(localStorage)); // the last ten puzzles finished
  const [pre, setPre] = useState([]); // premoves (uci) chosen while the opponent moves, played one by one on the person's turns

  const [startedAt, setStartedAt] = useState(0); // when the puzzle appeared (for the timer)
  const [elapsed, setElapsed] = useState(0); // seconds spent on it
  const timers = useRef([]);
  const autoTimer = useRef(null); // the countdown to the next puzzle: looking back stops it, the other timers go on
  const reported = useRef(false);
  const remembered = useRef(false); // the puzzle is already in the list of the last ones
  const hintUsed = useRef(false); // the hint was asked for on this puzzle (halves the points)
  const request = useRef(0);
  const currentId = useRef(""); // the puzzle on the board: a late answer about an older one must not touch the screen

  const later = useCallback((fn, ms) => {
    timers.current.push(setTimeout(fn, ms));
  }, []);
  const clearTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
    clearTimeout(autoTimer.current);
    autoTimer.current = null;
  }, []);
  const pushTrail = useCallback((fen, last, san) => setTrail((tr) => [...tr, { fen, last, san }]), []);
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
    (p, view = false) => {
      clearTimers();
      reported.current = view; // looking at a puzzle is no try
      remembered.current = view;
      hintUsed.current = false;
      currentId.current = p.id;
      setStartedAt(Date.now());
      setElapsed(0);
      setPuzzle(p);
      setTrail([{ fen: p.fen, last: null, san: null }]);
      setBack(null);
      setPre([]);
      setViewOnly(view);
      if (view) {
        // the whole solution at once, wound back to the position the puzzle starts from (after the opponent's first move)
        setTrail([{ fen: p.fen, last: null, san: null }, ...p.steps.map((s) => ({ fen: s.fen, last: squaresOf(s.uci), san: s.san }))]);
        setBack(1);
        setFen(p.steps[p.steps.length - 1].fen);
        setIdx(p.steps.length);
        setPhase("shown");
        setFailed(true);
        setLast(null);
        setBadge(null);
        setHint(null);
        setResult(null);
        setAuto(false);
        return;
      }
      setFen(p.fen);
      setIdx(0);
      setPhase("intro");
      setFailed(false);
      setLast(null);
      setBadge(null);
      setHint(null);
      setResult(null);
      setAuto(false);
      later(() => {
        const first = p.steps[0];
        setFen(first.fen);
        setLast(squaresOf(first.uci));
        pushTrail(first.fen, squaresOf(first.uci), first.san);
        sound(soundKindForSan(first.san));
        setIdx(1);
        setPhase("solving");
      }, INTRO_MS);
    },
    [clearTimers, later, sound, pushTrail]
  );

  const load = useCallback(
    async ({ view = false, ...opts } = {}) => {
      const mine = ++request.current;
      setLoading(true);
      setError("");
      try {
        if (!user) await guest(); // a guest account keeps the rating
        const d = await fetchPuzzle({ level: settings.puzzleLevel, ...opts });
        if (mine !== request.current) return;
        setYou(d.you || null);
        setPts(d.points || null);
        setDaily(d.daily || null);
        start(d.puzzle, view);
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
    const { mode: first, id, view } = fromHash();
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
    load(first === "daily" ? { daily: true } : { id, view });
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

  // The first try is reported once: a wrong move or the shown solution is a failure. The rated
  // puzzles move the rating and count the points in one step on the server; the others are practice
  // (the puzzle of the day keeps its record).
  function report(p, solved) {
    if (reported.current) return;
    reported.current = true;
    if (mode === "themes") {
      recordPractice(p.id, solved); // practice does not move the rating, but the trainer's plan counts it
      return;
    }
    if (mode === "daily") {
      sendDailyResult(solved).then(
        (r) => setDaily((d) => (d ? { ...d, status: r.status } : d)),
        (e) => setError(e.message)
      );
      return;
    }
    sendResult(p.id, solved, { hint: hintUsed.current }).then(
      (r) => {
        setYou(r.you); // the standing is kept even when the person has already moved on
        if (r.points) setPts(r.points);
        if (currentId.current === p.id) setResult(r);
      },
      (e) => setError(e.message)
    );
  }

  function flash(square, cls, ms) {
    setBadge({ square, cls });
    later(() => setBadge(null), ms);
  }

  // The puzzle is over: it goes to the top of the list of the last ten (kept in this browser), with how the first try went.
  function remember(p, ok) {
    if (remembered.current) return;
    remembered.current = true;
    setRecent((list) => {
      const next = addRecent(list, { id: p.id, mode, ok, rating: p.rating, at: Date.now() });
      saveRecent(localStorage, next);
      return next;
    });
  }

  // The puzzle is solved (also after a wrong try, which only cost the rating). The next one opens by itself (a setting, on by default).
  function markSolved() {
    setPhase("solved");
    remember(puzzle, !failed);
    if (!failed) report(puzzle, true);
    if (settings.puzzleAutoNext && mode !== "daily") {
      setAuto(true);
      autoTimer.current = setTimeout(() => act.current.next(), AUTO_MS);
    }
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
    pushTrail(j.fen, squaresOf(uci), j.san);
    setHint(null);
    sound(soundKindForSan(j.san));
    flash(to, "best", 900);
    if (j.done) {
      markSolved();
      return { ok: true };
    }
    // the opponent answers
    const reply = puzzle.steps[idx + 1];
    setIdx(idx + 1);
    setPhase("reply");
    later(() => {
      setFen(reply.fen);
      setLast(squaresOf(reply.uci));
      pushTrail(reply.fen, squaresOf(reply.uci), reply.san);
      sound(soundKindForSan(reply.san));
      if (idx + 2 >= puzzle.steps.length) {
        markSolved(); // the line ends with the opponent's move
      } else {
        setIdx(idx + 2);
        setPhase("solving");
      }
    }, REPLY_MS);
    return { ok: true };
  }

  // The first premove is played the moment it is the person's turn, if it is a legal move then; one that is
  // not drops the whole queue (the later ones were chosen on a position that did not come about). A wrong
  // move that is legal counts as any wrong move does. The others wait for their turns.
  useEffect(() => {
    if (!pre.length) return;
    if (phase === "solved" || phase === "shown") {
      setPre([]);
      return;
    }
    if (phase !== "solving") return;
    const play = premoveToPlay(legalFor(puzzle, idx), pre[0]);
    if (play) {
      setBack(null);
      setPre(onPlayerMove(play).ok ? pre.slice(1) : []);
    } else {
      setPre([]);
    }
  }, [phase, idx]); // eslint-disable-line react-hooks/exhaustive-deps

  function showHint() {
    hintUsed.current = true;
    setHint(hintSquare(puzzle, idx));
  }

  function showSolution() {
    clearTimers();
    setPre([]);
    setHint(null);
    setBadge(null);
    setPhase("shown");
    remember(puzzle, false);
    if (!failed) {
      setFailed(true);
      report(puzzle, false);
    }
    puzzle.steps.slice(idx).forEach((s, i) => {
      later(() => {
        setFen(s.fen);
        setLast(squaresOf(s.uci));
        pushTrail(s.fen, squaresOf(s.uci), s.san);
        sound(soundKindForSan(s.san));
      }, SHOW_MS * (i + 1));
    });
  }

  const finished = phase === "solved" || phase === "shown";
  const canNext = finished && !loading;
  const canSkip = !finished && !loading && mode !== "daily" && phase !== "loading";
  const canHint = phase === "solving" && hint === null;
  const canSolution = phase === "solving";

  // Winding the moves back and forth. Looking at an earlier position stops the countdown to the next puzzle
  // and keeps the board from taking moves; the last position is the current one.
  function go(i) {
    const n = Math.max(0, Math.min(trail.length - 1, i));
    if (autoTimer.current) {
      clearTimeout(autoTimer.current);
      autoTimer.current = null;
      setAuto(false);
    }
    setBack(n >= trail.length - 1 ? null : n);
  }

  // A puzzle of the list opens in the rated tab, to look at: the whole solution, the board on the position it starts from.
  function openView(id) {
    if (loading) return;
    setMode("rated");
    window.history.replaceState(null, "", `#puzzles/${id}/view`);
    load({ id, view: true });
  }
  // The puzzle being looked at, to solve for real this time (it is no try for the rating if it was tried before).
  function solveAgain() {
    window.history.replaceState(null, "", `#puzzles/${puzzle.id}`);
    load({ id: puzzle.id });
  }

  // The next puzzle (or, after the puzzle of the day, the other puzzles).
  function next() {
    if (!canNext) return;
    if (mode === "daily") chooseMode("rated");
    else load(mode === "themes" ? { theme } : {});
  }
  function skip() {
    if (canSkip) load(mode === "themes" ? { theme } : {});
  }

  // Keys: Enter, Space or N open the next puzzle, H asks for a hint, S shows the solution. A key
  // pressed in a field, or Enter and Space on a focused button or link (which press it themselves), is left alone.
  const act = useRef({});
  act.current = { next, showHint, showSolution, canNext, canHint, canSolution, go, at: back ?? trail.length - 1 };
  useEffect(() => {
    const onKey = (e) => {
      if (e.ctrlKey || e.metaKey || e.altKey || e.repeat) return;
      const el = e.target;
      if (el?.closest?.("input, textarea, select, [contenteditable='true']")) return;
      const what = keyAction(e.key);
      if (!what) return;
      const a = act.current;
      if (what === "next") {
        if ((e.key === "Enter" || e.key === " ") && el?.closest?.("button, a, [role='tab']")) return;
        if (!a.canNext) return;
        e.preventDefault();
        a.next();
      } else if (what === "hint") {
        if (a.canHint) a.showHint();
      } else if (what === "solution") {
        if (a.canSolution) a.showSolution();
      } else {
        e.preventDefault();
        a.go(what === "first" ? 0 : what === "last" ? Infinity : what === "back" ? a.at - 1 : a.at + 1);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  if (status === "unavailable") {
    return (
      <Placeholder>
        <div className="banner banner--error">{t("puzzlesPage.4")}</div>
      </Placeholder>
    );
  }
  if (!puzzle) {
    return <Placeholder>{error ? <div className="banner banner--error">{error}</div> : t("puzzleJs.6")}</Placeholder>;
  }

  const mySide = puzzle.orientation;
  const viewSide = settings.whiteBottom ? "white" : mySide; // "white always at the bottom"
  const looking = back !== null && trail[back];
  const shownFen = looking ? trail[back].fen : fen;
  const boardState = { fen: shownFen, legalMoves: phase === "solving" && !looking ? legalFor(puzzle, idx) : [] };
  // the analysis board opens on the position the puzzle sets (after the opponent's first move), not on where it ended up
  const analysisHref = finished ? `#analysis/${encodeFen(puzzle.steps[0].fen)}` : null;
  const say = viewOnly ? { kind: t("pzrecent.viewKind"), text: t("pzrecent.viewText"), tone: "" } : coachSay({ loading, phase, failed, idx, mySide });
  const view =
    mode === "rated"
      ? scoreView({ pointsMain, ratingVisible: settings.puzzleRating, you, pts, result })
      : scoreView({ pointsMain: false, ratingVisible: settings.puzzleRating, you, pts: null, result: null });
  const nextLabel = mode === "daily" ? t("puzzlesPage.6") : t("puzzlesPage.7");

  return (
    <Shell
      className="pz--puzzles"
      mode={mode}
      icon="puzzles"
      title={t(TITLES[mode])}
      head={
        settings.puzzleTimer && (
          <span className="pz__clock" title={t("puzzlesPage.8")} aria-label={t("puzzlesPage.8")}>
            {Math.floor(elapsed / 60)}:{String(elapsed % 60).padStart(2, "0")}
          </span>
        )
      }
      board={
        <Board
          state={boardState}
          orientation={viewSide}
          disabled={phase !== "solving" || !!looking}
          lastMove={looking ? trail[back].last : last}
          hint={looking ? null : hint}
          badge={looking ? null : badge}
          onPlayerMove={onPlayerMove}
          premove={{
            enabled: settings.premoveOn && phase === "reply" && !looking, // only while the opponent answers: before his first move the position is not the puzzle's yet
            side: colourLetter(mySide),
            queue: pre,
            onSet: (uci) => setPre((q) => [...q, uci].slice(0, MAX_PREMOVES)),
            onClear: () => setPre([]),
          }}
        />
      }
      bottom={
        <>
          <BigButton onClick={next} disabled={!canNext} auto={auto && canNext} ms={AUTO_MS}>
            <NextLabel>{nextLabel}</NextLabel>
          </BigButton>
          <div className="pz__tools">
            <button type="button" className="btn btn--ghost" onClick={showHint} disabled={!canHint} title={t("puzzlesPage.9")}>
              <Icon name="star" size={16} /> {t("puzzlesPage.10")}
            </button>
            <button type="button" className="btn btn--ghost" onClick={showSolution} disabled={!canSolution} title={t("puzzlesPage.11")}>
              <Icon name="check" size={16} /> {t("puzzlesPage.12")}
            </button>
            {viewOnly && (
              <button type="button" className="btn btn--ghost" onClick={solveAgain} disabled={loading}>
                {t("pzrecent.again")}
              </button>
            )}
            {mode !== "daily" && !viewOnly && (
              <button type="button" className="btn btn--ghost" onClick={skip} disabled={!canSkip} title={t("puzzlesPage.13")}>
                {t("puzzlesPage.14")}
              </button>
            )}
          </div>
          <p className="pz__keys">{t("puzzlesPage.15")}</p>
        </>
      }
    >
      {error && <div className="banner banner--error">{error}</div>}

      {mode === "daily" && daily && (
        <div className="pz__score">
          <span className="pz__label">{t("menu.puzzlesDaily")}</span>
          <span className="pz__num pz__num--text">{dayLabel(daily.date)}</span>
          <span className="pz__sub">
            {daily.status === "solved" && t("puzzlesPage.16")}
            {daily.status === "failed" && t("puzzlesPage.17")}
            {daily.status === "new" && t("puzzlesPage.18")}
          </span>
        </div>
      )}

      <Score view={view} />

      <RecentList list={recent} current={viewOnly ? puzzle.id : ""} onOpen={openView} />

      <Say kind={say.kind} tone={say.tone}>
        {say.text}
      </Say>

      <PlyNav trail={trail} at={back ?? trail.length - 1} onGo={go} analysisHref={analysisHref} />

      {result && !result.counted && (
        <p className="pz__meta">{t("puzzlesPage.19")}</p>
      )}
      {failed && !result && phase !== "shown" && mode === "rated" && <p className="pz__meta">{t("puzzlesPage.20")}</p>}
      {finished && mode === "themes" && (
        <p className="pz__meta">{t("puzzlesPage.21")}</p>
      )}

      <p className="pz__meta">{finished ? t("puzzlesPage.22", { rating: puzzle.rating }) : t("puzzlesPage.23")}</p>
      {(finished || settings.puzzleThemes) && (
        <div className="pz__themes">
          {ideaThemes(puzzle.themes).map((t) => (
            <span key={t} className="pz__chip">
              {themeName(t)}
            </span>
          ))}
        </div>
      )}

      {mode === "themes" && (
        <div className="pz__field">
          <label htmlFor="puzzle-theme">{t("puzzlesPage.24")}</label>
          <select id="puzzle-theme" className="field" value={theme} onChange={(e) => chooseTheme(e.target.value)} disabled={loading}>
            {themes.map((t) => (
              <option key={t.id} value={t.id}>
                {themeName(t.id)} ({t.count})
              </option>
            ))}
          </select>
        </div>
      )}

      {view?.main && user?.kind === "guest" && (
        <p className="pz__meta">
          {mode === "rated" ? t("puzzlesPage.25") : t("puzzlesPage.26")}
        </p>
      )}
    </Shell>
  );
}

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Board from "./components/Board.jsx";
import Captured from "./components/Captured.jsx";
import Icon from "./ui/Icon.jsx";
import { useMoveSound, useSettings } from "./settings/SettingsContext.jsx";
import MoveLog from "./components/MoveLog.jsx";
import Segmented from "./ui/Segmented.jsx";
import GameSummary from "./play/GameSummary.jsx";
import { START_FEN, buildPgn, describeResult, sanMoves } from "./play/game.js";
import { useAuth } from "./auth/AuthContext.jsx";
import ReviewInline from "./review/ReviewInline.jsx";
import { playSound } from "./sound/sound.js";
import * as engine from "./engine/dyanisEngine.js";
import { openingName, openingTitle } from "./openings/names.js";
import { tr, useT } from "./i18n/index.js";
import "./App.css";

const DEFAULT_DEPTH = 4;
const DEFAULT_MOVETIME_MS = 800;
const MIN_DEPTH = 1;
const MAX_DEPTH = 30;
// Past this depth a search without a time limit can run for minutes.
const SLOW_DEPTH = 12;

const clampDepth = (n) => Math.min(MAX_DEPTH, Math.max(MIN_DEPTH, Math.round(n)));
const SIDE_NAME = { white: "gameRoom.sideWhite", black: "gameRoom.sideBlack" };
const BOT_NAME = "Dyanis Bot";

export default function App() {
  const t = useT();
  const playMoveSound = useMoveSound();
  const { settings, save } = useSettings();
  const { user } = useAuth();
  const [state, setState] = useState(null);
  const [error, setError] = useState(null);
  const [thinking, setThinking] = useState(false);
  const [playerSide, setPlayerSide] = useState("white");
  const [bookStatus, setBookStatus] = useState({ loaded: false, entries: 0 });
  const [lastMove, setLastMove] = useState(null);
  const [engineNote, setEngineNote] = useState("");
  // Who resigned ("white" | "black"), or null. The engine's own status stays
  // "ongoing", so the page treats a resignation as the end of the game itself.
  const [resigned, setResigned] = useState(null);
  const [confirmResign, setConfirmResign] = useState(false);
  // The position the game began from (null: the normal start); needed to write its PGN.
  const [startFen, setStartFen] = useState(null);
  const [opening, setOpening] = useState({ opening: null, book: false }); // the opening reached so far
  const [showSummary, setShowSummary] = useState(false);
  const [reviewPgn, setReviewPgn] = useState(null); // set: the analysis replaces the game screen

  // Search strength controls. depth is always the max-depth ceiling;
  // when useMovetime is on, the engine uses iterative deepening with
  // movetimeMs as a time budget instead of always searching to a
  // fixed depth — same relationship -depth/-movetime have in the
  // CLI's -play (see cmd/cli/main.go). Turning useMovetime off means
  // "search exactly to `depth`, however long that takes" — safe to do
  // even at high depth, since the search runs in a Web Worker and
  // can no longer freeze the page (see engine/dyanisEngine.js), and a
  // search that takes too long can be abandoned (resign / new game).
  const [depth, setDepth] = useState(DEFAULT_DEPTH);
  const [depthText, setDepthText] = useState(String(DEFAULT_DEPTH)); // what is typed, before it is valid
  const [useMovetime, setUseMovetime] = useState(true);
  const [movetimeMs, setMovetimeMs] = useState(DEFAULT_MOVETIME_MS);

  // Custom-position loading: a plain FEN text field. Kept as its own
  // piece of state (not derived from `state.fen`) so typing/pasting
  // into it doesn't fight with the board's own current position —
  // it's only read when "Загрузить FEN" is actually clicked.
  const [fenInput, setFenInput] = useState("");

  // Bumped whenever a running search stops mattering (new game, resignation,
  // undo, leaving the page): the answer of an older search is then dropped.
  const searchId = useRef(0);

  const refreshBook = useCallback(() => {
    engine
      .bookInfoInWorker()
      .then(setBookStatus)
      .catch(() => {
        // воркер ещё не готов — не критично, просто пропускаем
      });
  }, []);

  useEffect(() => {
    let cancelled = false;
    engine
      .initEngine()
      .then(() => {
        if (cancelled) return;
        setState(engine.getState());
        refreshBook();
      })
      .catch((e) => setError(String(e)));
    return () => {
      cancelled = true;
      searchId.current++;
      engine.cancelSearch(); // do not keep a CPU core busy for a page nobody sees
    };
  }, [refreshBook]);

  // Abandons the search in progress, if any.
  const stopSearch = useCallback(() => {
    searchId.current++;
    engine.cancelSearch();
    setThinking(false);
  }, []);

  const triggerEngineIfNeeded = useCallback(
    async (freshState, side = playerSide) => {
      if (!freshState || freshState.status !== "ongoing") return;
      const engineToMove =
        (freshState.sideToMove === "white" && side === "black") ||
        (freshState.sideToMove === "black" && side === "white");
      if (!engineToMove) return;

      const mine = ++searchId.current;
      setThinking(true);
      try {
        const budget = useMovetime ? movetimeMs : 0;
        // Runs in a Web Worker — the page stays fully responsive no
        // matter how long this takes. No "let React paint first" hack
        // needed here (unlike a synchronous main-thread call would
        // require): awaiting a worker call never blocks anything.
        const decision = await engine.engineMoveAsync(freshState.fen, depth, budget);
        if (mine !== searchId.current) return; // abandoned meanwhile
        if (!decision.ok) {
          setError(decision.error);
          return;
        }
        // The worker only DECIDED the move; committing it to the
        // actual tracked game state always goes through the main
        // thread's own synchronous makeMove, same as a human move.
        const result = engine.makeMove(decision.uci);
        if (result.ok) {
          setState(result.state);
          setLastMove({ from: decision.uci.slice(0, 2), to: decision.uci.slice(2, 4) });
          playMoveSound(decision.san);
          setEngineNote(decision.fromBook ? tr("enginePage.3", { san: decision.san }) : tr("enginePage.4", { san: decision.san }));
          setError(null);
        } else {
          setError(result.error);
        }
      } catch (e) {
        if (!e?.cancelled && mine === searchId.current) setError(String(e));
      } finally {
        if (mine === searchId.current) setThinking(false);
      }
    },
    [playerSide, depth, useMovetime, movetimeMs, playMoveSound]
  );

  function handlePlayerMove(uci) {
    const result = engine.makeMove(uci);
    if (result.ok) {
      setState(result.state);
      setLastMove({ from: uci.slice(0, 2), to: uci.slice(2, 4) });
      playMoveSound(result.san);
      setEngineNote("");
      setError(null);
      triggerEngineIfNeeded(result.state); // fire-and-forget
    } else {
      setError(result.error);
    }
    return result;
  }

  function resetGameView() {
    setLastMove(null);
    setEngineNote("");
    setError(null);
    setResigned(null);
    setConfirmResign(false);
  }

  function handleNewGame() {
    stopSearch();
    const s = engine.newGame();
    setState(s);
    setStartFen(null);
    resetGameView();
    triggerEngineIfNeeded(s);
  }

  // Same position, colours swapped.
  function handleRematch() {
    const side = playerSide === "white" ? "black" : "white";
    stopSearch();
    const s = engine.newGame();
    setPlayerSide(side);
    setState(s);
    setStartFen(null);
    resetGameView();
    triggerEngineIfNeeded(s, side);
  }

  function handleResign() {
    stopSearch();
    setConfirmResign(false);
    setResigned(playerSide);
    setEngineNote("");
    if (settings.sound) playSound("end", settings.soundTheme);
  }

  // jsNewGame (see cmd/wasm/main.go) returns either a full stateResponse
  // (success — no "ok" field at all) or {ok:false, error} (bad FEN) —
  // unlike the no-arg call in handleNewGame above, which is always the
  // starting position and can't fail, this one has to check which shape
  // came back before treating the result as a position to render.
  function handleLoadPosition() {
    const fen = fenInput.trim();
    if (!fen) return;
    stopSearch();
    const s = engine.newGame(fen);
    if (s.ok === false) {
      setError(tr("enginePage.5", { error: s.error }));
      return;
    }
    setState(s);
    setStartFen(fen);
    resetGameView();
    triggerEngineIfNeeded(s);
  }

  // Pulls the FEN of the position currently on the board into the
  // input field — handy for saving a position you just reached (e.g.
  // right before the engine's move you want to re-examine) so you can
  // reload it later, at a different depth, without replaying the game.
  function handleCopyCurrentFen() {
    if (state) setFenInput(state.fen);
  }

  function handleUndo() {
    const result = engine.undo();
    if (result.ok) {
      setState(result.state);
      setLastMove(null);
      setEngineNote("");
      setResigned(null); // taking a move back reopens a resigned game
      setConfirmResign(false);
    }
  }

  function handleSwitchSide(side) {
    setPlayerSide(side);
    if (state && !resigned) triggerEngineIfNeeded(state, side);
  }

  function commitDepth(text) {
    const n = Number(text);
    const next = Number.isFinite(n) && text.trim() !== "" ? clampDepth(n) : depth;
    setDepth(next);
    setDepthText(String(next));
  }

  function onDepthChange(text) {
    setDepthText(text);
    const n = Number(text);
    if (text.trim() !== "" && Number.isFinite(n) && n >= MIN_DEPTH && n <= MAX_DEPTH) setDepth(Math.round(n));
  }

  async function handleLoadBook() {
    try {
      const res = await fetch(`${import.meta.env.BASE_URL}assets/gm2001.bin`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const buf = await res.arrayBuffer();
      await engine.loadBookInWorker(new Uint8Array(buf));
      refreshBook();
    } catch (e) {
      setError(tr("enginePage.6", { e }));
    }
  }

  async function handleClearBook() {
    await engine.clearBookInWorker();
    refreshBook();
  }

  // The opening is named live, by the server's ECO reference, after every move
  // of a game from the normal start (a custom position has no opening).
  const logText = state?.log || "";
  useEffect(() => {
    if (startFen) {
      setOpening({ opening: null, book: false });
      return undefined;
    }
    const sans = sanMoves(logText);
    if (sans.length === 0 || sans.length > 120) {
      setOpening({ opening: null, book: false });
      return undefined;
    }
    let alive = true;
    const t = setTimeout(() => {
      fetch("/api/opening?san=" + encodeURIComponent(sans.join(",")))
        .then((r) => (r.ok ? r.json() : null))
        .then((o) => alive && o && setOpening({ opening: o.opening, book: !!o.book }))
        .catch(() => {}); // no server (the page also works offline): no opening line
    }, 120);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [logText, startFen]);

  const info = useMemo(
    () =>
      state ? describeResult({ status: state.status, sideToMove: state.sideToMove, resigned, playerSide }) : null,
    [state, resigned, playerSide]
  );
  // The card opens when a game ends and goes away when it is taken back.
  const ended = info?.result ?? null;
  const endedReason = info?.reason ?? null;
  useEffect(() => {
    setShowSummary(ended !== null);
  }, [ended, endedReason, state?.fen]);

  // The review opens in place of the game, so the page does not jump.
  function handleReview() {
    setReviewPgn(
      buildPgn({
        sans: sanMoves(state.log),
        startFen: startFen || START_FEN,
        result: info.result,
        white: playerSide === "white" ? playerName : BOT_NAME,
        black: playerSide === "black" ? playerName : BOT_NAME,
      })
    );
    setShowSummary(false);
  }

  if (!state) {
    return (
      <div className="page page--loading">
        {error ? <p className="banner banner--error">{error}</p> : <p>{t("enginePage.7")}</p>}
      </div>
    );
  }

  if (reviewPgn) {
    return (
      <ReviewInline pgn={reviewPgn} finalFen={state.fen} orientation={playerSide} onClose={() => setReviewPgn(null)} />
    );
  }

  const gameOn = state.status === "ongoing" && !resigned;
  const center = settings.playLayout === "center";
  const playerName = user?.name || t("botsPage.8");
  const moveCount = sanMoves(state.log).length;
  const winner = resigned ? (resigned === "white" ? "black" : "white") : null;

  const statusBlock = (
    <div className="status">
      <p className="status__turn">
        {t("enginePage.9")} <strong>{state.sideToMove === "white" ? t("gameRoom.sideWhite") : t("gameRoom.sideBlack")}</strong>
      </p>
      {resigned && (
        <p className="status__flag status__flag--warn">
          {t("enginePage.10")} {t(SIDE_NAME[winner])}
        </p>
      )}
      {!resigned && state.inCheck && state.status === "ongoing" && (
        <p className="status__flag status__flag--warn">{t("enginePage.11")}</p>
      )}
      {state.status === "checkmate" && <p className="status__flag status__flag--warn">{t("gameEnd.5")}</p>}
      {state.status === "stalemate" && <p className="status__flag status__flag--warn">{t("enginePage.13")}</p>}
      {state.status === "draw-fifty-move" && <p className="status__flag status__flag--warn">{t("enginePage.14")}</p>}
      {state.status === "draw-repetition" && (
        <p className="status__flag status__flag--warn">{t("enginePage.15")}</p>
      )}
      {thinking && <p className="status__flag status__flag--thinking">{t("enginePage.16")}</p>}
      {opening.opening && (
        <p className="status__opening" title={openingTitle(opening.opening)}>
          <span className="review__eco">{opening.opening.eco}</span> {openingName(opening.opening)}
          {opening.book && <span className="status__theory">{t("gameRoom.51")}</span>}
        </p>
      )}
      {engineNote && <p className="status__note">{engineNote}</p>}
      {info && !showSummary && (
        <button className="btn btn--block status__summary" onClick={() => setShowSummary(true)}>
          {t("gameRoom.71")}
        </button>
      )}
    </div>
  );

  // "White always at the bottom": the board and the bars follow the side shown at the bottom.
  const viewSide = settings.whiteBottom ? "white" : playerSide;
  const engBar = (
    <div className="pbar">
      <span className="play__ico">
        <Icon name="bot" size={20} />
      </span>
      <div className="pbar__who">
        <span className="pbar__name">{BOT_NAME}</span>
        <span className="pbar__sub">
          {playerSide === "white" ? t("online.black") : t("online.white")}
          <Captured fen={state.fen} side={playerSide === "white" ? "black" : "white"} />
        </span>
      </div>
    </div>
  );
  const youBar = (
    <div className="pbar">
      <span className="play__ico">
        <Icon name="user" size={20} />
      </span>
      <div className="pbar__who">
        <span className="pbar__name">{playerName}</span>
        <span className="pbar__sub">
          {playerSide === "white" ? t("online.white") : t("online.black")}
          <Captured fen={state.fen} side={playerSide} />
        </span>
      </div>
    </div>
  );

  const controls = (
    <div className="controls">
      <div className="controls__main">
        <button className="btn btn--primary" onClick={handleNewGame}>
          {t("gameRoom.newGame")}
        </button>
        {confirmResign ? (
          <span className="controls__confirm">
            <button className="btn btn--danger" onClick={handleResign}>
              {t("gameRoom.68")}
            </button>
            <button className="btn btn--ghost" onClick={() => setConfirmResign(false)}>
              {t("gameRoom.67")}
            </button>
          </span>
        ) : (
          <button className="btn btn--danger-ghost" onClick={() => (settings.confirmGiveUp ? setConfirmResign(true) : handleResign())} disabled={!gameOn}>
            {t("gameRoom.69")}
          </button>
        )}
      </div>
      <button className="btn" onClick={handleUndo} disabled={thinking}>
        {t("enginePage.25")}
      </button>

      <div className="side-toggle">
        <label>
          <input type="radio" name="side" checked={playerSide === "white"} onChange={() => handleSwitchSide("white")} />
          {t("enginePage.26")}
        </label>
        <label>
          <input type="radio" name="side" checked={playerSide === "black"} onChange={() => handleSwitchSide("black")} />
          {t("enginePage.27")}
        </label>
      </div>

      <div className="depth-controls">
        <label className="depth-controls__row">
          {t("enginePage.28")}
          <input
            type="number"
            className="field"
            min={MIN_DEPTH}
            max={MAX_DEPTH}
            step={1}
            value={depthText}
            onChange={(e) => onDepthChange(e.target.value)}
            onBlur={(e) => commitDepth(e.target.value)}
          />
        </label>

        <label className="depth-controls__row">
          <input type="checkbox" checked={useMovetime} onChange={(e) => setUseMovetime(e.target.checked)} />
          {t("enginePage.29")}
        </label>

        {useMovetime && (
          <label className="depth-controls__row">
            {t("enginePage.30")}
            <input
              type="number"
              className="field"
              min={100}
              step={100}
              value={movetimeMs}
              onChange={(e) => setMovetimeMs(Math.max(100, Number(e.target.value) || 100))}
            />
          </label>
        )}

        <p className="depth-controls__hint">
          {useMovetime
            ? t("enginePage.31", { depth, movetimeMs })
            : t("enginePage.32", { depth })}
          {!useMovetime && depth >= SLOW_DEPTH
            ? t("enginePage.33")
            : ""}
        </p>
      </div>

      <div className="fen-controls">
        <label className="fen-controls__row">
          FEN:
          <input
            type="text"
            className="field fen-controls__input"
            value={fenInput}
            onChange={(e) => setFenInput(e.target.value)}
            placeholder={t("enginePage.34")}
            disabled={thinking}
          />
        </label>
        <div className="fen-controls__buttons">
          <button className="btn" onClick={handleLoadPosition} disabled={thinking || !fenInput.trim()}>
            {t("enginePage.35")}
          </button>
          <button className="btn" onClick={handleCopyCurrentFen} disabled={thinking}>
            {t("enginePage.36")}
          </button>
        </div>
      </div>

      <div className="book-controls">
        <button className="btn" onClick={handleLoadBook} disabled={bookStatus.loaded}>
          {t("enginePage.37")}
        </button>
        <button className="btn" onClick={handleClearBook} disabled={!bookStatus.loaded}>
          {t("enginePage.38")}
        </button>
        <span className="book-controls__status">
          {bookStatus.loaded ? t("enginePage.39", { entries: bookStatus.entries }) : t("enginePage.40")}
        </span>
      </div>
    </div>
  );

  return (
    <div className={`play play--${center ? "center" : "side"}`}>
      <header className="play__head">
        <div>
          <h1 className="page__title">{t("enginePage.title")}</h1>
          <p className="page__sub">{t("enginePage.42")}</p>
        </div>
        <Segmented
          label={t("enginePage.43")}
          value={center ? "center" : "side"}
          onChange={(v) => save({ ...settings, playLayout: v })}
          options={[
            ["side", t("enginePage.44")],
            ["center", t("enginePage.45")],
          ]}
        />
      </header>

      {error && <div className="banner banner--error">{error}</div>}

      <div className="play__grid">
        <div className="play__board">
          {viewSide === playerSide ? engBar : youBar}
          <Board
            state={state}
            orientation={viewSide}
            disabled={thinking || !gameOn}
            lastMove={lastMove}
            onPlayerMove={handlePlayerMove}
          />
          {viewSide === playerSide ? youBar : engBar}
          {info && showSummary && (
            <GameSummary
              info={info}
              white={playerSide === "white" ? playerName : BOT_NAME}
              black={playerSide === "black" ? playerName : BOT_NAME}
              you={playerSide}
              moves={moveCount}
              signedIn={!!user}
              onReview={handleReview}
              onNew={handleNewGame}
              onRematch={handleRematch}
              onClose={() => setShowSummary(false)}
            />
          )}
        </div>

        {center && (
          <aside className="play__left card">
            {statusBlock}
            <MoveLog log={state.log} />
          </aside>
        )}

        <aside className="play__side card">
          {!center && statusBlock}
          {controls}
          {!center && <MoveLog log={state.log} />}
        </aside>
      </div>
    </div>
  );
}

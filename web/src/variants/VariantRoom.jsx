import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChessboardDnDProvider } from "react-chessboard";
import Board from "../components/Board.jsx";
import { fenFromBoard } from "../components/boardrules.js";
import { GoogleLogo } from "../auth/Avatar.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import GameSummary from "../play/GameSummary.jsx";
import Icon from "../ui/Icon.jsx";
import Chat from "../online/Chat.jsx";
import { PlayerBar, PlayerName } from "../online/GameRoom.jsx";
import { describeTC } from "../online/clock.js";
import { beepDue } from "../online/lowtime.js";
import { useClocks, useGame } from "../online/useGame.js";
import { GAME_API, createReview, getGame, inviteUrl, joinGame } from "./api.js";
import { describeEnd, groupTurns, isBoardMove, lastMoveOf, listRows, mergeClasses } from "./describe.js";
import { getVariant } from "./registry.js";
import BughouseRoom from "./bughouse/BughouseRoom.jsx";
import { tx, useT } from "../i18n/index.js";
import "../online/online.css";
import "./variants.css";

const SIDE_GEN = { white: "gameRoom.genWhite", black: "gameRoom.genBlack" }; // "ход белых"
const SIDE_INS = { white: "gameRoom.insWhite", black: "gameRoom.insBlack" }; // "играете белыми"

// How the duck (a piece of no side, code "xD") looks unless the variant draws it itself: a plain disc.
function Disc({ squareWidth }) {
  return (
    <svg width={squareWidth} height={squareWidth} viewBox="0 0 100 100" aria-hidden="true">
      <circle cx="50" cy="50" r="30" style={{ fill: "var(--warn)", stroke: "var(--side-black-2)", strokeWidth: 4 }} />
    </svg>
  );
}
const DEFAULT_PIECES = { xD: Disc };

// What a definition without a page of its own looks like: the server knows a variant this build of the
// site does not (an old page after an update); the usual board and list play it well enough.
const fallbackDef = (game) => ({ id: game.variant, name: tx(game.variantName) || game.variant, short: "", rules: [] });

function MoveListing({ moves, def }) {
  const t = useT();
  const end = useRef(null);
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: "nearest" });
  }, [moves.length]);
  const rows = listRows(groupTurns(moves));
  const cell = (turn) => (turn ? turn.items.map((m) => (def.moveText ? def.moveText(m) : m.san)).join(def.moveSep ?? " ") : "");
  return (
    <div className="moves" aria-label={t("gameRoom.10")}>
      {rows.length === 0 && <p className="moves__empty">{t("gameRoom.11")}</p>}
      {rows.map(([w, b], i) => (
        <div key={i} className="moves__row vmoves__row">
          <span className="moves__no">{i + 1}.</span>
          <span className="moves__san">{cell(w)}</span>
          <span className="moves__san">{cell(b)}</span>
        </div>
      ))}
      <div ref={end} />
    </div>
  );
}

// Waiting for a friend: the address to send.
function InviteCard({ id, game, onCancel }) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const url = inviteUrl(id);
  const input = useRef(null);
  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
    } catch {
      input.current?.select();
      document.execCommand?.("copy");
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <section className="card invite">
      <h2>{t("gameRoom.12")}</h2>
      <p className="invite__sub">
        {tx(game.variantName)}, {describeTC(game.tc)}{t("vroom.8")}
      </p>
      <div className="invite__row">
        <input ref={input} className="field" readOnly value={url} onFocus={(e) => e.target.select()} aria-label={t("gameRoom.15")} />
        <button type="button" className="btn btn--primary" onClick={copy}>
          {copied ? t("gameRoom.16") : t("gameRoom.17")}
        </button>
      </div>
      <button type="button" className="btn btn--ghost" onClick={onCancel}>
        {t("gameRoom.18")}
      </button>
    </section>
  );
}

// Someone opened the link of a game that waits for a second player.
function JoinCard({ id, game }) {
  const t = useT();
  const { user, google, guest } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const host = game.white || game.black;
  const mySide = game.white ? "black" : "white";

  async function join() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest();
      await joinGame(id);
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <section className="card invite">
      <h2>{host?.name} {t("gameRoom.19")}</h2>
      <p className="invite__sub">
        {tx(game.variantName)}, {describeTC(game.tc)}{t("gameRoom.20")} {t(SIDE_INS[mySide])}.
      </p>
      {error && <div className="banner banner--error">{error}</div>}
      <button type="button" className="btn btn--primary btn--block" onClick={join} disabled={busy}>
        {busy ? t("gameRoom.21") : user ? t("botsPage.play") : t("gameRoom.23")}
      </button>
      {!user && google && (
        <a className="btn btn--google btn--block" href={`/api/auth/google/login?next=${encodeURIComponent(`#variants/game/${id}`)}`}>
          <GoogleLogo size={16} /> {t("auth.signInGoogle")}
        </a>
      )}
      <p className="invite__hint">{t("gameRoom.25")}</p>
    </section>
  );
}

// A game of a chess variant: the room of internal/online with the position as an object, the actions of
// the variant (drops, the duck, ...) given to its slots, and no premoves and no analysis.
// What a variant's slots receive, `ctx`:
//   game      the whole snapshot of vplay;  view = game.position (board, turn, legal, last, check, extra)
//   you       "white" | "black" | "" (a spectator);  orientation: the side at the bottom of the screen
//   playing   the game is going on;  myTurn: it is the viewer's turn to act
//   legal     the actions the viewer may take now (the same as view.legal, [] when not their turn)
//   act(a)    sends an action (the string State.Apply takes); false when the socket is not open
//   ui, setUi(patch)   the variant's own state of the screen (a chosen piece of the pocket, the first click of a
//             two-click action); it is emptied after every action and at every new position
//   settings  the person's settings
// A game of four (Bughouse) has its own room; the kind of a game is learnt from one read of its snapshot, so the
// hooks of the room of two stay unconditional.
export default function VariantRoom({ id }) {
  const t = useT();
  const [kind, setKind] = useState(""); // "" until known, then "four" or "two"
  useEffect(() => {
    let alive = true;
    getGame(id).then(
      (m) => alive && setKind(Array.isArray(m?.game?.players) ? "four" : "two"),
      () => alive && setKind("two") // not found, or the server is busy: the room of two says so and retries
    );
    return () => {
      alive = false;
    };
  }, [id]);
  if (!kind) return <div className="page room room--loading">{t("gameRoom.29")}</div>;
  return kind === "four" ? <BughouseRoom id={id} /> : <TwoSeatRoom id={id} />;
}

function TwoSeatRoom({ id }) {
  const t = useT();
  const { user } = useAuth();
  const { settings } = useSettings();
  const { snap, status, notice, rev, send } = useGame(id, user?.id, GAME_API);
  const clocks = useClocks(snap);
  const [showSummary, setShowSummary] = useState(true);
  const [confirmResign, setConfirmResign] = useState(false);
  const [confirmDraw, setConfirmDraw] = useState(false);
  const [ui, setUiState] = useState({});
  const [reviewBusy, setReviewBusy] = useState(false);
  const [reviewError, setReviewError] = useState("");

  const game = snap?.game;
  const you = snap?.you || "";
  const moves = game?.moves || [];
  const view = game?.position;
  const def = useMemo(() => (game ? getVariant(game.variant) || fallbackDef(game) : null), [game?.variant, game?.variantName]); // eslint-disable-line react-hooks/exhaustive-deps

  // The review of a finished game by the engine (a definition with review: true, now Chess960): the server makes it
  // (or finds the one already made) and the page of reviews opens it.
  async function openReview() {
    setReviewBusy(true);
    setReviewError("");
    try {
      window.location.hash = `review/${await createReview(id)}`;
    } catch (e) {
      setReviewError(e.message);
      setReviewBusy(false);
    }
  }

  const setUi = useCallback((patch) => setUiState((u) => ({ ...u, ...patch })), []);
  // a new position or the end of the game drops what the person had half done
  useEffect(() => {
    setUiState({});
  }, [moves.length, game?.status, game?.turn]);

  // Sounds: a move that arrives, and the end of the game. The first state after opening the page is only remembered.
  const seen = useRef(null); // { moves, status }
  useEffect(() => {
    if (!game) return;
    const prev = seen.current;
    seen.current = { moves: moves.length, status: game.status };
    if (!prev || !settings.sound) return;
    if (game.status === "ended" && prev.status !== "ended") {
      playSound("end", settings.soundTheme);
    } else if (moves.length > prev.moves) {
      playSound(soundKindForSan(moves[moves.length - 1].san || ""), settings.soundTheme);
    }
  }, [game, moves, settings.sound, settings.soundTheme]);

  // The summary opens again whenever a game ends (also after a rematch).
  const ended = game?.status === "ended";
  useEffect(() => setShowSummary(true), [ended, id]);
  useEffect(() => {
    setConfirmResign(false);
    setConfirmDraw(false);
  }, [game?.status, moves.length]);

  // The warning of the clock: a beep at ten seconds and in each of the last three.
  const myIdx = you === "white" ? 0 : you === "black" ? 1 : -1;
  const mySec = clocks && myIdx >= 0 ? Math.ceil(clocks[myIdx] / 1000) : null;
  const lastSec = useRef(null);
  useEffect(() => {
    const prev = lastSec.current;
    lastSec.current = mySec;
    if (prev === null || mySec === null) return;
    if (settings.timeWarning && settings.sound && game?.status === "playing" && game.turn === you && beepDue(prev, mySec)) {
      playSound("lowtime", settings.soundTheme);
    }
  }, [mySec]); // eslint-disable-line react-hooks/exhaustive-deps

  // Both agreed to a rematch: go to the new game.
  const nextId = game?.rematch?.id;
  useEffect(() => {
    if (nextId && you) window.location.hash = `variants/game/${nextId}`;
  }, [nextId, you]);

  const legal = useMemo(() => (view?.legal && game?.status === "playing" ? view.legal : []), [view?.legal, game?.status]);
  const boardState = useMemo(
    () => ({ board: view?.board || {}, legalMoves: legal.filter(isBoardMove).map((uci) => ({ uci })) }),
    [view?.board, legal]
  );
  const pieces = useMemo(() => (def?.pieces ? { ...DEFAULT_PIECES, ...def.pieces } : DEFAULT_PIECES), [def]);
  const checkClasses = useMemo(() => Object.fromEntries((view?.check || []).map((sq) => [sq, "sq-check"])), [view?.check]);

  if (status === "notfound") {
    return (
      <div className="page room">
        <div className="banner banner--error">{t("gameRoom.27")}</div>
        <a className="btn btn--primary" href="#variants">
          {t("vroom.toVariants")}
        </a>
      </div>
    );
  }
  if (!game) return <div className="page room room--loading">{t("gameRoom.29")}</div>;

  const playing = game.status === "playing";
  const myTurn = playing && !!you && game.turn === you;
  // Someone about to join sees the board from the side they would play.
  const joinsAsBlack = !you && game.status === "waiting" && !!game.white;
  const orientation = settings.whiteBottom ? "white" : you === "black" || joinsAsBlack ? "black" : "white";
  const top = orientation === "white" ? "black" : "white";
  const bottom = orientation === "white" ? "white" : "black";
  const clockOf = (side) => (clocks ? clocks[side === "white" ? 0 : 1] : null);
  const present = (side) => (side === "white" ? game.presence.white : game.presence.black);
  const act = (action) => send("move", { action });
  const ctx = { game, view, you, orientation, playing, myTurn, legal: myTurn ? legal : [], act, ui, setUi, settings, notice };
  const bp = (def.boardProps && def.boardProps(ctx)) || {};
  // def.boardOf(ctx) may give the board to draw instead of the snapshot's (blindfold hides it, or peeks)
  const shownBoard = def.boardOf?.(ctx);
  const materialFen = def.captured === false ? undefined : fenFromBoard(view.board);

  const bar = (side) => (
    <PlayerBar
      player={game[side]}
      side={side}
      ms={clockOf(side)}
      running={playing && game.turn === side}
      present={present(side)}
      you={you === side}
      fen={materialFen}
      warn={settings.timeWarning}
    />
  );
  // A variant with a pocket (def.onSpareDrop) lets pieces be dragged from outside the board: the board and the
  // pocket then share one drag-and-drop provider of react-chessboard.
  const Dnd = def.onSpareDrop ? ChessboardDnDProvider : Fragment;
  const Top = def.PanelTop;
  const Bottom = def.PanelBottom;
  const SideCard = def.Side;
  const Controls = def.Controls;

  const turns = groupTurns(moves);
  const info = ended ? describeEnd({ result: game.result, termination: game.termination, you, reasons: def.reasons }) : null;
  const rm = game.rematch;
  const rematch = you
    ? {
        label: rm?.offer === you ? t("gameRoom.37") : rm?.offer ? t("gameRoom.38") : t("gameRoom.39"),
        onClick: () => send("rematch"),
      }
    : undefined;

  const earlyAbort = playing && turns.length < 2;
  const drawFromOpponent = playing && game.drawOffer && game.drawOffer !== you;
  const drawFromMe = playing && game.drawOffer === you;
  const firstMoveLeft = game.firstMoveBy ? Math.max(0, Math.ceil((game.firstMoveBy - Date.now()) / 1000)) : 0;
  const hint = playing && myTurn && def.hint ? tx(def.hint(ctx)) : "";

  let statusLine = "";
  if (game.status === "waiting") statusLine = t("gameRoom.40");
  else if (playing) statusLine = myTurn ? t("gameRoom.41") : you ? t("gameRoom.42") : t("gameRoom.43", { p: t(SIDE_GEN[game.turn]) });
  else if (game.status === "aborted") statusLine = t("gameRoom.44");
  else if (ended) statusLine = info.title === t("onlineGames.2") ? t("onlineGames.2") : `${info.subtitle}`;

  return (
    <div className="room vroom" data-variant={game.variant}>
      {status === "reconnecting" && <div className="banner banner--error room__net">{t("gameRoom.46")}</div>}
      <Dnd>
      <div className="room__grid">
        <div className="room__main">
          {bar(top)}
          {Top && <Top {...ctx} />}
          <div className="room__board">
            <Board
              key={rev}
              state={shownBoard ? { ...boardState, board: shownBoard } : boardState}
              orientation={orientation}
              disabled={!myTurn || !!def.boardLocked?.(ctx)}
              lastMove={def.lastMove ? def.lastMove(ctx) : lastMoveOf(view.last)}
              onPlayerMove={(uci) => ({ ok: act(uci) })}
              confirmMove={settings.confirmMove && myTurn}
              arrows={bp.arrows}
              badge={bp.badge}
              hint={bp.hint}
              onSquare={def.onSquare ? (square) => !!def.onSquare(square, ctx) : undefined}
              extraDots={bp.extraDots}
              squareClasses={mergeClasses(checkClasses, bp.squareClasses)}
              extraPieces={pieces}
              onSpareDrop={def.onSpareDrop ? (piece, square) => def.onSpareDrop(piece, square, ctx) : undefined}
            />
            {ended && showSummary && info && (
              <GameSummary
                info={info}
                white={<PlayerName player={game.white} fallback={t("online.white")} />}
                black={<PlayerName player={game.black} fallback={t("online.black")} />}
                you={you}
                moves={turns.length}
                signedIn={!!user}
                newLabel={t("menu.variants")}
                rematch={rematch}
                hint={def.review ? tx(def.reviewHint) : t("vroomx.36", { variantName: tx(game.variantName) })}
                onReview={def.review ? openReview : undefined}
                busy={reviewBusy}
                error={reviewError}
                onNew={() => (window.location.hash = `variants/${game.variant}`)}
                onClose={() => setShowSummary(false)}
              />
            )}
          </div>
          {Bottom && <Bottom {...ctx} />}
          {bar(bottom)}
        </div>

        <aside className="room__side">
          <section className="card room__status">
            <p className="room__state">{statusLine}</p>
            <p className="room__meta">
              {tx(game.variantName)}, {describeTC(game.tc)}
              {game.presence.watching > 0 ? t("gameRoom.50", { watching: game.presence.watching }) : ""}
            </p>
            {hint && <p className="room__meta vroom__hint">{hint}</p>}
            {notice && <p className="room__notice">{notice}</p>}
            {playing && firstMoveLeft > 0 && turns.length < 2 && (
              <p className="room__meta">{t("gameRoom.55")}</p>
            )}
            {!you && playing && <p className="room__meta">{t("gameRoom.56")}</p>}
          </section>

          {SideCard && <SideCard {...ctx} />}

          {game.status === "waiting" && you && <InviteCard id={id} game={game} onCancel={() => send("abort")} />}
          {game.status === "waiting" && !you && <JoinCard id={id} game={game} />}

          {game.status === "aborted" && (
            <section className="card room__status">
              <p className="room__meta">{t("gameRoom.57")}</p>
              <a className="btn btn--primary" href={`#variants/${game.variant}`}>
                {t("menu.variants")}
              </a>
            </section>
          )}

          {drawFromOpponent && (
            <section className="card room__offer">
              <p>{t("gameRoom.63")}</p>
              <div className="room__row">
                <button type="button" className="btn btn--primary" onClick={() => send("draw")}>
                  {t("gameRoom.64")}
                </button>
                <button type="button" className="btn" onClick={() => send("drawDecline")}>
                  {t("gameRoom.65")}
                </button>
              </div>
            </section>
          )}

          {playing && you && (
            <section className="card room__actions">
              {Controls && <Controls {...ctx} />}
              {confirmDraw ? (
                <div className="room__row">
                  <button
                    type="button"
                    className="btn btn--primary"
                    onClick={() => {
                      setConfirmDraw(false);
                      send("draw");
                    }}
                  >
                    {t("gameRoom.66")}
                  </button>
                  <button type="button" className="btn btn--ghost" onClick={() => setConfirmDraw(false)}>
                    {t("gameRoom.67")}
                  </button>
                </div>
              ) : confirmResign ? (
                <div className="room__row">
                  <button type="button" className="btn btn--danger" onClick={() => send("resign")}>
                    {t("gameRoom.68")}
                  </button>
                  <button type="button" className="btn btn--ghost" onClick={() => setConfirmResign(false)}>
                    {t("gameRoom.67")}
                  </button>
                </div>
              ) : (
                <div className="room__row">
                  <button type="button" className="btn btn--danger-ghost" onClick={() => (settings.confirmGiveUp ? setConfirmResign(true) : send("resign"))}>
                    <Icon name="flag" size={16} /> {t("gameRoom.69")}
                  </button>
                  {drawFromMe ? (
                    <button type="button" className="btn" onClick={() => send("drawDecline")}>
                      {t("gameRoom.70")}
                    </button>
                  ) : (
                    <button type="button" className="btn" onClick={() => (settings.confirmGiveUp ? setConfirmDraw(true) : send("draw"))} disabled={turns.length < 2}>
                      {t("onlineGames.2")}
                    </button>
                  )}
                </div>
              )}
              {earlyAbort && (
                <button type="button" className="btn btn--ghost" onClick={() => send("abort")}>
                  {t("gameRoom.18")}
                </button>
              )}
            </section>
          )}

          {ended && def.review && moves.length > 0 && (
            <section className="card room__status vroom__review">
              <button type="button" className="btn btn--primary btn--block" onClick={openReview} disabled={reviewBusy}>
                <Icon name="review" size={18} />
                {reviewBusy ? t("vroom.review") : t("menu.review")}
              </button>
              {reviewError && !showSummary && <p className="room__notice">{reviewError}</p>}
              {def.reviewHint && <p className="room__meta">{tx(def.reviewHint)}</p>}
            </section>
          )}

          {ended && !showSummary && (
            <button type="button" className="btn btn--block" onClick={() => setShowSummary(true)}>
              {t("gameRoom.71")}
            </button>
          )}

          {def.MovesCard ? (
            // a variant may own the card of the moves (blindfold hides the list); the list is its child
            <def.MovesCard {...ctx} moves={moves}>
              <MoveListing moves={moves} def={def} />
            </def.MovesCard>
          ) : (
            <section className="card room__moves">
              <h2>{t("gameRoom.72")}</h2>
              <MoveListing moves={moves} def={def} />
            </section>
          )}

          {game.white && game.black && (game.status === "playing" || game.status === "ended") && <Chat game={game} you={you} send={send} />}
        </aside>
      </div>
      </Dnd>
    </div>
  );
}

import { useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Avatar, { GoogleLogo } from "../auth/Avatar.jsx";
import BotAvatar from "../bots/BotAvatar.jsx";
import { botText } from "../bots/botText.js";
import { useBots } from "../bots/api.js";
import { useAuth } from "../auth/AuthContext.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../sound/sound.js";
import GameSummary from "../play/GameSummary.jsx";
import { describeFinished } from "../play/game.js";
import ReviewInline from "../review/ReviewInline.jsx";
import Icon from "../ui/Icon.jsx";
import Captured from "../components/Captured.jsx";
import Chat from "./Chat.jsx";
import MoveTime from "./MoveTime.jsx";
import { fetchPgn, inviteUrl, joinGame } from "./api.js";
import { describeTC, formatClock } from "./clock.js";
import { fetchCorrespondence } from "../community/api.js";
import { timeLeft, urgent } from "../community/logic.js";
import { changeLine, ratingText } from "./rating.js";
import { userHash } from "../leaders/leaders.js";
import { LOW_MS, beepDue } from "./lowtime.js";
import { colourLetter, premoveToPlay } from "../components/boardrules.js";
import { useClocks, useGame } from "./useGame.js";
import { openingName, openingTitle } from "../openings/names.js";
import { tOpts, useT } from "../i18n/index.js";
import "./online.css";

const MAX_PREMOVES = 30; // how many premoves may wait
const SIDE_NAME = { white: "gameRoom.sideWhite", black: "gameRoom.sideBlack" };
const SIDE_GEN = { white: "gameRoom.genWhite", black: "gameRoom.genBlack" }; // "ход белых"
const SIDE_INS = { white: "gameRoom.insWhite", black: "gameRoom.insBlack" }; // "играете белыми"

// A person's name links to their public profile; a bot, or a seat with no account, is plain text.
export function PlayerName({ player, fallback }) {
  const t = useT();
  if (!player) return fallback;
  if (player.bot || !player.id || player.id.startsWith("bot:")) return player.bot ? botText(t, player.bot, "name", player.name) : player.name;
  return (
    <a className="pbar__link" href={userHash(player.id)}>
      {player.name}
    </a>
  );
}

export function PlayerBar({ player, side, ms, running, present, you, fen, warn }) {
  const t = useT();
  const low = warn && running && ms !== null && ms < LOW_MS;
  return (
    <div className={`pbar${running ? " pbar--turn" : ""}`}>
      {player?.bot ? (
        <BotAvatar bot={{ id: player.bot, name: player.name }} size={36} />
      ) : player ? (
        <Avatar user={{ name: player.name, avatarUrl: player.avatar }} size={36} />
      ) : (
        <span className="pbar__empty" />
      )}
      <div className="pbar__who">
        <span className="pbar__name">
          <PlayerName player={player} fallback={t("gameRoom.7")} />
          {player?.elo ? <span className="pbar__elo"> ({ratingText(player.elo, player.prov)})</span> : null}
          {you && <span className="pbar__you">{t("leadersPage.13")}</span>}
        </span>
        <span className="pbar__sub">
          <span className={`pbar__dot${player && present ? " pbar__dot--on" : ""}`} aria-hidden="true" />
          {t(SIDE_NAME[side])}
          {player && !present ? t("gameRoom.9") : ""}
          {fen && <Captured fen={fen} side={side} />}
        </span>
      </div>
      {ms !== null && <span className={`pbar__clock${running ? " pbar__clock--on" : ""}${low ? " pbar__clock--low" : ""}`}>{formatClock(ms)}</span>}
    </div>
  );
}

function MoveListing({ moves }) {
  const t = useT();
  const end = useRef(null);
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: "nearest" });
  }, [moves.length]);
  const rows = [];
  for (let i = 0; i < moves.length; i += 2) rows.push([moves[i], moves[i + 1]]);
  return (
    <div className="moves" aria-label={t("gameRoom.10")}>
      {rows.length === 0 && <p className="moves__empty">{t("gameRoom.11")}</p>}
      {rows.map(([w, b], i) => (
        <div key={i} className="moves__row">
          <span className="moves__no">{i + 1}.</span>
          <span className="moves__san">
            {w.san}
            <MoveTime ms={w.ms} />
          </span>
          <span className="moves__san">
            {b?.san || ""}
            {b && <MoveTime ms={b.ms} />}
          </span>
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
        {t("gameRoom.13")} {describeTC(game.tc)}{t("gameRoom.14")}
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
function JoinCard({ id, game, onJoined }) {
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
      onJoined();
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return (
    <section className="card invite">
      <h2>{host?.name} {t("gameRoom.19")}</h2>
      <p className="invite__sub">
        {t("gameRoom.13")} {describeTC(game.tc)}{t("gameRoom.20")} {t(SIDE_INS[mySide])}.
      </p>
      {error && <div className="banner banner--error">{error}</div>}
      <button type="button" className="btn btn--primary btn--block" onClick={join} disabled={busy}>
        {busy ? t("gameRoom.21") : user ? t("botsPage.play") : t("gameRoom.23")}
      </button>
      {!user && google && (
        <a className="btn btn--google btn--block" href={`/api/auth/google/login?next=${encodeURIComponent(`#online/${id}`)}`}>
          <GoogleLogo size={16} /> {t("auth.signInGoogle")}
        </a>
      )}
      <p className="invite__hint">{t("gameRoom.25")}</p>
    </section>
  );
}

// backHash / backLabel: where "a new game" leads (the lobby, or the list of bots).
export default function GameRoom({ id, backHash = "online", backLabel }) {
  const t = useT();
  const { user } = useAuth();
  const { settings } = useSettings();
  const { snap, status, notice, rev, send } = useGame(id, user?.id);
  const clocks = useClocks(snap);
  const [showSummary, setShowSummary] = useState(true);
  const [confirmResign, setConfirmResign] = useState(false);
  const [confirmDraw, setConfirmDraw] = useState(false);
  const [pre, setPre] = useState([]); // premoves (uci) waiting their turn, played one by one
  const [reviewBusy, setReviewBusy] = useState(false);
  const [reviewError, setReviewError] = useState("");
  const [reviewPgn, setReviewPgn] = useState(null); // set: the analysis replaces the game screen
  const botsInfo = useBots(user?.id); // the bots' lines, and the person's progress against them

  const game = snap?.game;
  const you = snap?.you || "";
  // A correspondence game counts days: the time left is shown to the minute and looked at again every half minute.
  const [nowMs, setNowMs] = useState(() => Date.now());
  const corr = !!game?.tc?.days;
  useEffect(() => {
    if (!corr) return undefined;
    setNowMs(Date.now());
    const t = setInterval(() => setNowMs(Date.now()), 30000);
    return () => clearInterval(t);
  }, [corr]);
  const moves = game?.moves || [];

  // A game against a bot: who it is, what it says, and what the first win earned.
  const botSeat = game?.white?.bot ? game.white : game?.black?.bot ? game.black : null;
  const botMeta = botSeat ? botsInfo.data?.bots.find((b) => b.id === botSeat.bot) : null;
  const winsBefore = useRef(null); // wins against this bot when the game was still going
  const endedNow = game?.status === "ended";
  useEffect(() => {
    if (!botSeat || !botsInfo.data) return;
    if (winsBefore.current === null && !endedNow) winsBefore.current = botsInfo.data.progress[botSeat.bot]?.wins || 0;
  }, [botSeat, botsInfo.data, endedNow]);
  // The server writes the result a moment after the game ends: look again.
  useEffect(() => {
    if (!botSeat || !endedNow || winsBefore.current === null) return undefined;
    const t = [setTimeout(botsInfo.reload, 700), setTimeout(botsInfo.reload, 2500)];
    return () => t.forEach(clearTimeout);
  }, [botSeat, endedNow]); // eslint-disable-line react-hooks/exhaustive-deps

  // Sounds: a move that arrives, and the end of the game. The first state
  // after opening the page is only remembered, not played.
  const seen = useRef(null); // { moves, status }
  useEffect(() => {
    if (!game) return;
    const prev = seen.current;
    seen.current = { moves: moves.length, status: game.status };
    if (!prev || !settings.sound) return;
    if (game.status === "ended" && prev.status !== "ended") {
      playSound("end", settings.soundTheme);
    } else if (moves.length === prev.moves + 1) {
      playSound(soundKindForSan(moves[moves.length - 1].san), settings.soundTheme);
    }
  }, [game, moves, settings.sound, settings.soundTheme]);

  // The summary opens again whenever a game ends (also after a rematch).
  const ended = game?.status === "ended";
  useEffect(() => setShowSummary(true), [ended, id]);
  useEffect(() => {
    setConfirmResign(false);
    setConfirmDraw(false);
  }, [game?.status, moves.length]);

  // The first premove is played as soon as it is the person's turn, if it is legal then; the others
  // wait for their turns. One that is not legal drops the whole queue (the later ones were chosen
  // on a position that did not come about).
  useEffect(() => {
    if (!pre.length) return;
    if (game?.status !== "playing") {
      setPre([]);
      return;
    }
    if (game.turn === you) {
      const play = premoveToPlay(game.legal || [], pre[0]);
      if (play) {
        setPre(pre.slice(1));
        send("move", { uci: play });
      } else {
        setPre([]);
      }
    }
  }, [game?.status, game?.turn, moves.length]); // eslint-disable-line react-hooks/exhaustive-deps

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

  // "Show the review after the game": it opens by itself once, when a game the person played has ended.
  const autoOpened = useRef("");
  useEffect(() => {
    if (!settings.autoReview || game?.status !== "ended" || !you || moves.length < 2 || autoOpened.current === id) return;
    autoOpened.current = id;
    fetchPgn(id).then(
      (pgn) => {
        setReviewPgn(pgn);
        setShowSummary(false);
      },
      () => {}
    );
  }, [settings.autoReview, game?.status, you, moves.length, id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Both agreed to a rematch: go to the new game.
  const nextId = game?.rematch?.id;
  useEffect(() => {
    if (nextId && you) window.location.hash = `${backHash}/${nextId}`;
  }, [nextId, you, backHash]);

  const boardState = useMemo(
    () => ({ fen: game?.fen, legalMoves: (game?.legal || []).map((uci) => ({ uci })) }),
    [game?.fen, game?.legal]
  );

  if (status === "notfound") {
    return (
      <div className="page room">
        <div className="banner banner--error">{t("gameRoom.27")}</div>
        <a className="btn btn--primary" href="#online">
          {t("gameRoom.28")}
        </a>
      </div>
    );
  }
  if (!game) return <div className="page room room--loading">{t("gameRoom.29")}</div>;

  if (reviewPgn) {
    return (
      <ReviewInline
        pgn={reviewPgn}
        finalFen={game.fen}
        orientation={you === "black" ? "black" : "white"}
        onClose={() => setReviewPgn(null)}
      />
    );
  }

  const playing = game.status === "playing";
  const myTurn = playing && you && game.turn === you;
  // Someone about to join sees the board from the side they would play.
  const joinsAsBlack = !you && game.status === "waiting" && !!game.white;
  const orientation = settings.whiteBottom ? "white" : you === "black" || joinsAsBlack ? "black" : "white";
  const top = orientation === "white" ? "black" : "white";
  const bottom = orientation === "white" ? "white" : "black";
  const lastUci = moves.length ? moves[moves.length - 1].uci : "";
  const lastMove = lastUci ? { from: lastUci.slice(0, 2), to: lastUci.slice(2, 4) } : null;
  const clockOf = (side) => (clocks ? clocks[side === "white" ? 0 : 1] : null);
  const present = (side) => (game[side]?.bot ? true : side === "white" ? game.presence.white : game.presence.black);
  const bar = (side) => (
    <PlayerBar
      player={game[side]}
      side={side}
      ms={clockOf(side)}
      running={playing && game.turn === side}
      present={present(side)}
      you={you === side}
      fen={game.fen}
      warn={settings.timeWarning}
    />
  );

  function onPlayerMove(uci) {
    const ok = send("move", { uci });
    // "After a move go to the next game": the next correspondence game that waits for a move of mine.
    if (ok && corr && settings.corrNextGame) {
      setTimeout(() => {
        fetchCorrespondence().then(
          (list) => {
            const next = list.find((g) => g.yourTurn && g.id !== id);
            if (next) window.location.hash = `online/${next.id}`;
          },
          () => {},
        );
      }, 700);
    }
    return { ok };
  }

  // The review opens in place of the game, so the page does not jump.
  async function review() {
    setReviewBusy(true);
    setReviewError("");
    try {
      setReviewPgn(await fetchPgn(id));
      setShowSummary(false);
    } catch (e) {
      setReviewError(e.message);
    } finally {
      setReviewBusy(false);
    }
  }

  const info = ended ? describeFinished({ result: game.result, termination: game.termination, you }) : null;

  // What the first win over a bot earned (once the server has written it down).
  let reward = "";
  if (botSeat && ended && info?.outcome === "win" && winsBefore.current !== null) {
    const now = botsInfo.data?.progress[botSeat.bot]?.wins || 0;
    if (now > winsBefore.current) {
      reward =
        winsBefore.current === 0
          ? t("gameRoom.30", { p: botMeta?.points ?? 0 })
          : t("gameRoom.31");
    }
  }
  // What a rated game did to the rating of the person who played it.
  if (!botSeat && ended && game.rated && you) {
    const line = changeLine(game.ratingChange?.[you]);
    if (line) reward = t("gameRoom.32", { line });
    else reward = game.ratingChange ? t("gameRoom.33") : t("gameRoom.34");
  }
  // The bot's line: a greeting at the start, a remark at the end.
  let botLine = "";
  if (botMeta) {
    if (ended) {
      botLine = info?.outcome === "win" ? botText(t, botMeta.id, "onLose", botMeta.onLose) : info?.outcome === "loss" ? botText(t, botMeta.id, "onWin", botMeta.onWin) : t("gameRoom.35");
    } else if (game.status === "playing") {
      botLine = moves.length < 2 ? botText(t, botMeta.id, "greeting", botMeta.greeting) : game.turn !== you ? t("gameRoom.36") : "";
    }
  }
  const rm = game.rematch;
  const rematch = you
    ? {
        label: rm?.offer === you ? t("gameRoom.37") : rm?.offer ? t("gameRoom.38") : t("gameRoom.39"),
        onClick: () => send("rematch"),
      }
    : undefined;

  const earlyAbort = playing && moves.length < 2;
  const drawFromOpponent = playing && game.drawOffer && game.drawOffer !== you;
  const drawFromMe = playing && game.drawOffer === you;
  const firstMoveLeft = game.firstMoveBy ? Math.max(0, Math.ceil((game.firstMoveBy - Date.now()) / 1000)) : 0;

  let statusLine = "";
  if (game.status === "waiting") statusLine = t("gameRoom.40");
  else if (playing) statusLine = myTurn ? t("gameRoom.41") : you ? t("gameRoom.42") : t("gameRoom.43", { p: t(SIDE_GEN[game.turn]) });
  else if (game.status === "aborted") statusLine = t("gameRoom.44");
  else if (ended) statusLine = info.title === t("onlineGames.2") ? t("onlineGames.2") : `${info.subtitle}`;

  return (
    <div className="room">
      {status === "reconnecting" && <div className="banner banner--error room__net">{t("gameRoom.46")}</div>}
      <div className="room__grid">
        <div className="room__main">
          {bar(top)}
          <div className="room__board">
            <Board
              key={rev}
              state={boardState}
              orientation={orientation}
              disabled={!myTurn}
              lastMove={lastMove}
              onPlayerMove={onPlayerMove}
              confirmMove={(corr ? settings.corrConfirmMove : settings.confirmMove) && myTurn}
              premove={{
                enabled: settings.premoveOn && playing && !!you && !corr,
                side: colourLetter(you),
                queue: pre,
                onSet: (uci) => setPre((q) => [...q, uci].slice(0, MAX_PREMOVES)),
                onClear: () => setPre([]),
              }}
            />
            {ended && showSummary && info && (
              <GameSummary
                info={info}
                white={<PlayerName player={game.white} fallback={t("online.white")} />}
                black={<PlayerName player={game.black} fallback={t("online.black")} />}
                you={you}
                moves={moves.length}
                signedIn={!!user}
                busy={reviewBusy}
                error={reviewError}
                newLabel={backLabel}
                reward={reward}
                rematch={rematch}
                onReview={review}
                onNew={() => (window.location.hash = backHash)}
                onClose={() => setShowSummary(false)}
              />
            )}
          </div>
          {bar(bottom)}
        </div>

        <aside className="room__side">
          {botSeat && botMeta && (
            <section className="card botsay">
              <BotAvatar bot={botMeta} size={48} />
              <div className="botsay__text">
                <div className="botsay__name">
                  {botText(t, botMeta.id, "name", botMeta.name)}
                  <span>{botMeta.elo}</span>
                </div>
                {botLine && <p className="botsay__line">{botLine}</p>}
              </div>
            </section>
          )}
          <section className="card room__status">
            <p className="room__state">{statusLine}</p>
            <p className="room__meta">
              {game.rated ? t("gameRoom.49") : t("gameRoom.13")} {describeTC(game.tc)}
              {game.presence.watching > 0 ? t("gameRoom.50", { watching: game.presence.watching }) : ""}
            </p>
            {game.opening && (
              <p className="room__opening" title={openingTitle(game.opening)}>
                <span className="review__eco">{game.opening.eco}</span> {openingName(game.opening)}
                {game.book && <span className="room__theory">{t("gameRoom.51")}</span>}
              </p>
            )}
            {playing && corr && game.due > 0 && (
              <p className={`room__meta room__due${myTurn && urgent(game.due, nowMs, game.tc.days) ? " room__due--warn" : ""}`}>
                {myTurn ? t("gameRoom.52") : you ? t("gameRoom.53") : t("gameRoom.54", { p: t(SIDE_GEN[game.turn]) })}: {timeLeft(game.due, nowMs)}
              </p>
            )}
            {notice && <p className="room__notice">{notice}</p>}
            {playing && firstMoveLeft > 0 && moves.length < 2 && (
              <p className="room__meta">{t("gameRoom.55")}</p>
            )}
            {!you && playing && <p className="room__meta">{t("gameRoom.56")}</p>}
          </section>

          {game.status === "waiting" && you && <InviteCard id={id} game={game} onCancel={() => send("abort")} />}
          {game.status === "waiting" && !you && <JoinCard id={id} game={game} onJoined={() => {}} />}

          {game.status === "aborted" && (
            <section className="card room__status">
              <p className="room__meta">{t("gameRoom.57")}</p>
              <a className="btn btn--primary" href={"#" + backHash}>
                {backLabel ?? t("gameRoom.newGame")}
              </a>
            </section>
          )}

          {playing && corr && game.claimable && you && !myTurn && (
            <section className="card room__offer">
              <p>{t("gameRoom.58")}</p>
              <div className="room__row">
                <button type="button" className="btn btn--primary" onClick={() => send("claim")}>
                  {t("gameRoom.59")}
                </button>
              </div>
              <p className="room__meta">{t("gameRoom.60")}</p>
            </section>
          )}
          {playing && corr && game.claimable && myTurn && (
            <section className="card room__offer">
              <p>{t("gameRoom.61")}</p>
              <p className="room__meta">{t("gameRoom.62")}</p>
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
                    <button type="button" className="btn" onClick={() => (settings.confirmGiveUp ? setConfirmDraw(true) : send("draw"))} disabled={moves.length < 2}>
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

          {ended && !showSummary && (
            <button type="button" className="btn btn--block" onClick={() => setShowSummary(true)}>
              {t("gameRoom.71")}
            </button>
          )}

          <section className="card room__moves">
            <h2>{t("gameRoom.72")}</h2>
            <MoveListing moves={moves} />
          </section>

          {!botSeat && game.white && game.black && (game.status === "playing" || game.status === "ended") && (
            <Chat game={game} you={you} send={send} />
          )}
        </aside>
      </div>
    </div>
  );
}

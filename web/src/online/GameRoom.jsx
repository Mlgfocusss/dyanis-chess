import { useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import Avatar, { GoogleLogo } from "../auth/Avatar.jsx";
import BotAvatar from "../bots/BotAvatar.jsx";
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
import { fetchPgn, inviteUrl, joinGame } from "./api.js";
import { describeTC, formatClock } from "./clock.js";
import { useClocks, useGame } from "./useGame.js";
import "./online.css";

const SIDE_NAME = { white: "белые", black: "чёрные" };
const SIDE_GEN = { white: "белых", black: "чёрных" }; // "ход белых"
const SIDE_INS = { white: "белыми", black: "чёрными" }; // "играете белыми"

function PlayerBar({ player, side, ms, running, present, you, fen }) {
  const low = running && ms !== null && ms < 20000;
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
          {player ? player.name : "Ждём соперника…"}
          {player?.elo ? <span className="pbar__elo"> ({player.elo})</span> : null}
          {you && <span className="pbar__you">вы</span>}
        </span>
        <span className="pbar__sub">
          <span className={`pbar__dot${player && present ? " pbar__dot--on" : ""}`} aria-hidden="true" />
          {SIDE_NAME[side]}
          {player && !present ? ", не в сети" : ""}
          {fen && <Captured fen={fen} side={side} />}
        </span>
      </div>
      {ms !== null && <span className={`pbar__clock${running ? " pbar__clock--on" : ""}${low ? " pbar__clock--low" : ""}`}>{formatClock(ms)}</span>}
    </div>
  );
}

function MoveListing({ moves }) {
  const end = useRef(null);
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: "nearest" });
  }, [moves.length]);
  const rows = [];
  for (let i = 0; i < moves.length; i += 2) rows.push([moves[i], moves[i + 1]]);
  return (
    <div className="moves" aria-label="Ходы партии">
      {rows.length === 0 && <p className="moves__empty">Ходов пока нет</p>}
      {rows.map(([w, b], i) => (
        <div key={i} className="moves__row">
          <span className="moves__no">{i + 1}.</span>
          <span className="moves__san">{w.san}</span>
          <span className="moves__san">{b?.san || ""}</span>
        </div>
      ))}
      <div ref={end} />
    </div>
  );
}

// Waiting for a friend: the address to send.
function InviteCard({ id, game, onCancel }) {
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
      <h2>Отправьте ссылку сопернику</h2>
      <p className="invite__sub">
        Партия {describeTC(game.tc)}. Как только соперник откроет ссылку и нажмёт «Играть», она начнётся.
      </p>
      <div className="invite__row">
        <input ref={input} className="field" readOnly value={url} onFocus={(e) => e.target.select()} aria-label="Ссылка-приглашение" />
        <button type="button" className="btn btn--primary" onClick={copy}>
          {copied ? "Скопировано" : "Копировать"}
        </button>
      </div>
      <button type="button" className="btn btn--ghost" onClick={onCancel}>
        Отменить партию
      </button>
    </section>
  );
}

// Someone opened the link of a game that waits for a second player.
function JoinCard({ id, game, onJoined }) {
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
      <h2>{host?.name} приглашает вас сыграть</h2>
      <p className="invite__sub">
        Партия {describeTC(game.tc)}. Вы играете {SIDE_INS[mySide]}.
      </p>
      {error && <div className="banner banner--error">{error}</div>}
      <button type="button" className="btn btn--primary btn--block" onClick={join} disabled={busy}>
        {busy ? "Входим в партию…" : user ? "Играть" : "Играть как гость"}
      </button>
      {!user && google && (
        <a className="btn btn--google btn--block" href={`/api/auth/google/login?next=${encodeURIComponent(`#online/${id}`)}`}>
          <GoogleLogo size={16} /> Войти через Google
        </a>
      )}
      <p className="invite__hint">Или просто смотрите: ссылка открывается без входа.</p>
    </section>
  );
}

// backHash / backLabel: where "a new game" leads (the lobby, or the list of bots).
export default function GameRoom({ id, backHash = "online", backLabel = "Новая партия" }) {
  const { user } = useAuth();
  const { settings } = useSettings();
  const { snap, status, notice, rev, send } = useGame(id, user?.id);
  const clocks = useClocks(snap);
  const [showSummary, setShowSummary] = useState(true);
  const [confirmResign, setConfirmResign] = useState(false);
  const [reviewBusy, setReviewBusy] = useState(false);
  const [reviewError, setReviewError] = useState("");
  const [reviewPgn, setReviewPgn] = useState(null); // set: the analysis replaces the game screen
  const botsInfo = useBots(user?.id); // the bots' lines, and the person's progress against them

  const game = snap?.game;
  const you = snap?.you || "";
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
  useEffect(() => setConfirmResign(false), [game?.status, moves.length]);

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
        <div className="banner banner--error">Такой партии нет. Возможно, ссылка неверна или партия давно удалена.</div>
        <a className="btn btn--primary" href="#online">
          Создать новую партию
        </a>
      </div>
    );
  }
  if (!game) return <div className="page room room--loading">Открываю партию…</div>;

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
  const orientation = you === "black" || joinsAsBlack ? "black" : "white";
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
    />
  );

  function onPlayerMove(uci) {
    return { ok: send("move", { uci }) };
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
          ? `Бот побеждён! +${botMeta?.points ?? 0} очков`
          : "Победа засчитана. Очки за этого бота вы уже получали.";
    }
  }
  // The bot's line: a greeting at the start, a remark at the end.
  let botLine = "";
  if (botMeta) {
    if (ended) {
      botLine = info?.outcome === "win" ? botMeta.onLose : info?.outcome === "loss" ? botMeta.onWin : "Ничья. Хорошая партия!";
    } else if (game.status === "playing") {
      botLine = moves.length < 2 ? botMeta.greeting : game.turn !== you ? "Думаю…" : "";
    }
  }
  const rm = game.rematch;
  const rematch = you
    ? {
        label: rm?.offer === you ? "Ждём ответа…" : rm?.offer ? "Принять реванш" : "Реванш",
        onClick: () => send("rematch"),
      }
    : undefined;

  const earlyAbort = playing && moves.length < 2;
  const drawFromOpponent = playing && game.drawOffer && game.drawOffer !== you;
  const drawFromMe = playing && game.drawOffer === you;
  const firstMoveLeft = game.firstMoveBy ? Math.max(0, Math.ceil((game.firstMoveBy - Date.now()) / 1000)) : 0;

  let statusLine = "";
  if (game.status === "waiting") statusLine = "Ждём второго игрока";
  else if (playing) statusLine = myTurn ? "Ваш ход" : you ? "Ход соперника" : `Ход ${SIDE_GEN[game.turn]}`;
  else if (game.status === "aborted") statusLine = "Партия отменена";
  else if (ended) statusLine = info.title === "Ничья" ? "Ничья" : `${info.subtitle}`;

  return (
    <div className="room">
      {status === "reconnecting" && <div className="banner banner--error room__net">Нет связи с сервером, подключаюсь заново…</div>}
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
            />
            {ended && showSummary && info && (
              <GameSummary
                info={info}
                white={game.white?.name || "Белые"}
                black={game.black?.name || "Чёрные"}
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
                  {botMeta.name}
                  <span>{botMeta.elo}</span>
                </div>
                {botLine && <p className="botsay__line">{botLine}</p>}
              </div>
            </section>
          )}
          <section className="card room__status">
            <p className="room__state">{statusLine}</p>
            <p className="room__meta">
              Партия {describeTC(game.tc)}
              {game.presence.watching > 0 ? ` · смотрят: ${game.presence.watching}` : ""}
            </p>
            {game.opening && (
              <p className="room__opening">
                <span className="review__eco">{game.opening.eco}</span> {game.opening.name}
                {game.book && <span className="room__theory">теория</span>}
              </p>
            )}
            {notice && <p className="room__notice">{notice}</p>}
            {playing && firstMoveLeft > 0 && moves.length < 2 && (
              <p className="room__meta">Партия отменится, если первый ход не сделан в течение минуты.</p>
            )}
            {!you && playing && <p className="room__meta">Вы наблюдаете за партией.</p>}
          </section>

          {game.status === "waiting" && you && <InviteCard id={id} game={game} onCancel={() => send("abort")} />}
          {game.status === "waiting" && !you && <JoinCard id={id} game={game} onJoined={() => {}} />}

          {game.status === "aborted" && (
            <section className="card room__status">
              <p className="room__meta">Партия не состоялась.</p>
              <a className="btn btn--primary" href={"#" + backHash}>
                {backLabel}
              </a>
            </section>
          )}

          {drawFromOpponent && (
            <section className="card room__offer">
              <p>Соперник предлагает ничью</p>
              <div className="room__row">
                <button type="button" className="btn btn--primary" onClick={() => send("draw")}>
                  Принять
                </button>
                <button type="button" className="btn" onClick={() => send("drawDecline")}>
                  Отклонить
                </button>
              </div>
            </section>
          )}

          {playing && you && (
            <section className="card room__actions">
              {confirmResign ? (
                <div className="room__row">
                  <button type="button" className="btn btn--danger" onClick={() => send("resign")}>
                    Да, сдаться
                  </button>
                  <button type="button" className="btn btn--ghost" onClick={() => setConfirmResign(false)}>
                    Нет
                  </button>
                </div>
              ) : (
                <div className="room__row">
                  <button type="button" className="btn btn--danger-ghost" onClick={() => setConfirmResign(true)}>
                    <Icon name="flag" size={16} /> Сдаться
                  </button>
                  {drawFromMe ? (
                    <button type="button" className="btn" onClick={() => send("drawDecline")}>
                      Отозвать ничью
                    </button>
                  ) : (
                    <button type="button" className="btn" onClick={() => send("draw")} disabled={moves.length < 2}>
                      Ничья
                    </button>
                  )}
                </div>
              )}
              {earlyAbort && (
                <button type="button" className="btn btn--ghost" onClick={() => send("abort")}>
                  Отменить партию
                </button>
              )}
            </section>
          )}

          {ended && !showSummary && (
            <button type="button" className="btn btn--block" onClick={() => setShowSummary(true)}>
              Итоги партии
            </button>
          )}

          <section className="card room__moves">
            <h2>Ходы</h2>
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

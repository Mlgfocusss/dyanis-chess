import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChessboardDnDProvider } from "react-chessboard";
import Board from "../../components/Board.jsx";
import { GoogleLogo } from "../../auth/Avatar.jsx";
import { useAuth } from "../../auth/AuthContext.jsx";
import { useSettings } from "../../settings/SettingsContext.jsx";
import { playSound, soundKindForSan } from "../../sound/sound.js";
import GameSummary from "../../play/GameSummary.jsx";
import Icon from "../../ui/Icon.jsx";
import Chat from "../../online/Chat.jsx";
import { PlayerBar } from "../../online/GameRoom.jsx";
import { describeTC } from "../../online/clock.js";
import { useGame } from "../../online/useGame.js";
import { GAME_API, inviteUrl, joinGame } from "../api.js";
import { groupTurns, isBoardMove, lastMoveOf, listRows, mergeClasses } from "../describe.js";
import { getVariant } from "../registry.js";
import Pocket from "../pockets/Pocket.jsx";
import { dropAction, dropClick, dropDots, droppableKinds, kindOfCode, pocketCounts, promotedClasses, sideOfCode } from "../pockets/pockets.js";
import { TEAM_SEATS, boardOrder, bottomSides, bugClocks, chatForRoom, describeBugEnd, movesOfBoard, partnerSeat, rematchLabel, seatBoard, seatLabel, seatOf, seatSide, teamNames, teamOf } from "./bug.js";
import { tx, useT } from "../../i18n/index.js";
import "../../online/online.css";
import "../variants.css";
import "../pockets/pockets.css";
import "./bughouse.css";

const OTHER = { white: "black", black: "white" };

function useMedia(query) {
  const get = () => typeof window !== "undefined" && window.matchMedia(query).matches;
  const [on, setOn] = useState(get);
  useEffect(() => {
    const m = window.matchMedia(query);
    const h = () => setOn(m.matches);
    m.addEventListener("change", h);
    h();
    return () => m.removeEventListener("change", h);
  }, [query]);
  return on;
}

// The time left of each of the four seats, ticking: the seats whose clock runs count down from when the state arrived.
function useBugClocks(snap) {
  const [, tick] = useState(0);
  const running = (snap?.game?.clock?.running?.length || 0) > 0;
  useEffect(() => {
    if (!running) return undefined;
    const t = setInterval(() => tick((n) => n + 1), 100);
    return () => clearInterval(t);
  }, [running, snap]);
  return bugClocks(snap?.game?.clock, snap?.receivedAt, performance.now());
}

// One board with its two players, their clocks and pockets. The player's own pocket is the only interactive one.
function BoardColumn({ game, board, yourSeat, bottom, active, mini, ui, setUi, act, rev, clocks, settings, onActivate }) {
  const t = useT();
  const pos = game.boards[board].position;
  const turn = game.boards[board].turn;
  const playing = game.status === "playing";
  const myBoard = yourSeat >= 0 ? seatBoard(yourSeat) : -1;
  const mine = board === myBoard;
  const mySide = yourSeat >= 0 ? seatSide(yourSeat) : "";
  const myTurn = playing && mine && turn === mySide;
  const legal = useMemo(() => (myTurn ? pos.legal || [] : []), [myTurn, pos.legal]);
  const top = OTHER[bottom];
  const boardId = `bug-board-${board}`;
  const drop = mine ? ui.drop || null : null;

  const state = useMemo(() => ({ board: pos.board || {}, legalMoves: legal.filter(isBoardMove).map((uci) => ({ uci })) }), [pos.board, legal]);
  const checkClasses = useMemo(() => Object.fromEntries((pos.check || []).map((sq) => [sq, "sq-check"])), [pos.check]);
  const classes = useMemo(() => mergeClasses(checkClasses, promotedClasses(pos.extra)), [checkClasses, pos.extra]);
  const dots = useMemo(() => (myTurn && drop ? dropDots(legal, drop) : undefined), [myTurn, drop, legal]);

  function onSquare(square) {
    if (!drop) return false;
    const action = myTurn ? dropClick(legal, drop, square) : null;
    setUi({ drop: null });
    if (!action) return false;
    act(action);
    return true;
  }
  function onSpareDrop(piece, square) {
    if (!myTurn || sideOfCode(piece) !== mySide) return;
    const action = dropAction(kindOfCode(piece), square);
    if (legal.includes(action)) act(action);
    else setUi({ drop: null });
  }

  const bar = (side) => {
    const seat = seatOf(board, side);
    return (
      <PlayerBar
        player={game.players[seat]}
        side={side}
        ms={clocks ? clocks[seat] : null}
        running={game.status === "playing" && turn === side}
        present={!!game.presence.seats[seat]}
        you={yourSeat === seat}
        warn={settings.timeWarning}
      />
    );
  };
  const pocket = (side) => {
    const seat = seatOf(board, side);
    const usable = myTurn && seat === yourSeat;
    return (
      <Pocket
        side={side}
        counts={pocketCounts(pos.extra, side)}
        interactive={usable}
        droppable={usable ? droppableKinds(legal) : undefined}
        selected={seat === yourSeat ? drop : null}
        onSelect={(k) => setUi({ drop: k })}
        boardId={boardId}
        label={t("vcrazy.1")}
        note={seat === yourSeat ? t("vcrazy.2") : yourSeat >= 0 && seat === partnerSeat(yourSeat) ? t("vbugroom.3") : undefined}
      />
    );
  };

  return (
    <section className={`bug__col${active ? " bug__col--active" : ""}${mini ? " bug__col--mini" : ""}${mine ? " bug__col--mine" : ""}`} data-board={board} aria-label={t("vbugroom.4", { p: board + 1 })}>
      <header className="bug__title">
        <span className="bug__name">{t("vbugroom.5")} {board + 1}</span>
        {mine && <span className="bug__tag">{t("vbugroom.6")}</span>}
        {mini && (
          <button type="button" className="btn btn--ghost bug__switch" onClick={onActivate}>
            {t("vbugroom.7")}
          </button>
        )}
      </header>
      {bar(top)}
      {pocket(top)}
      <div className="bug__board">
        <Board
          key={`${rev}-${board}`}
          boardId={boardId}
          state={state}
          orientation={bottom}
          disabled={!myTurn}
          lastMove={lastMoveOf(pos.last)}
          onPlayerMove={(uci) => ({ ok: act(uci) })}
          confirmMove={settings.confirmMove && myTurn}
          onSquare={mine ? onSquare : undefined}
          extraDots={dots}
          squareClasses={classes}
          onSpareDrop={mine ? onSpareDrop : undefined}
        />
      </div>
      {pocket(bottom)}
      {bar(bottom)}
    </section>
  );
}

function MovesCard({ moves }) {
  const t = useT();
  return (
    <section className="card room__moves bug__moves">
      <h2>{t("gameRoom.72")}</h2>
      <div className="bug__lists">
        {[0, 1].map((b) => {
          const rows = listRows(groupTurns(movesOfBoard(moves, b)));
          return (
            <div key={b} className="bug__list" data-board={b}>
              <h3 className="bug__listh">{t("vbugroom.5")} {b + 1}</h3>
              <div className="moves" aria-label={t("vbugroom.9", { p: b + 1 })}>
                {rows.length === 0 && <p className="moves__empty">{t("gameRoom.11")}</p>}
                {rows.map(([w, k], i) => (
                  <div key={i} className="moves__row">
                    <span className="moves__no">{i + 1}.</span>
                    <span className="moves__san">{w ? w.items.map((m) => m.san).join(" ") : ""}</span>
                    <span className="moves__san">{k ? k.items.map((m) => m.san).join(" ") : ""}</span>
                  </div>
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

// The four seats while the game waits: who sits where, a place to take, the link to send.
function SeatsCard({ id, game, yourSeat, send, onTake, busy, error }) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const input = useRef(null);
  const url = inviteUrl(id);
  const taken = game.players.filter(Boolean).length;
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
    <section className="card invite bug__seats" aria-label={t("vbugroom.11")}>
      <h2>{t("vbugroom.12")}</h2>
      <p className="invite__sub">
        {tx(game.variantName)}, {describeTC(game.tc)}{t("vbugroom.13")}{taken} {t("vbugroom.14")}
      </p>
      {error && <div className="banner banner--error">{error}</div>}
      {TEAM_SEATS.map((seats, team) => (
        <div key={team} className="bug__team">
          <h3 className="bug__teamh">{t("vbugroom.15")} {team + 1}</h3>
          {seats.map((s) => {
            const p = game.players[s];
            return (
              <div key={s} className="bug__seat" data-seat={s}>
                <span className="bug__seatl">{seatLabel(s)}</span>
                {p ? (
                  <span className="bug__seatwho">
                    {p.name}
                    {yourSeat === s && <span className="pbar__you">{t("leadersPage.13")}</span>}
                  </span>
                ) : (
                  <button type="button" className="btn bug__take" disabled={busy} onClick={() => onTake(s)}>
                    {yourSeat >= 0 ? t("vbugroom.17") : t("vbugroom.18")}
                  </button>
                )}
              </div>
            );
          })}
        </div>
      ))}
      <div className="invite__row">
        <input ref={input} className="field" readOnly value={url} onFocus={(e) => e.target.select()} aria-label={t("gameRoom.15")} />
        <button type="button" className="btn btn--primary" onClick={copy}>
          {copied ? t("gameRoom.16") : t("gameRoom.17")}
        </button>
      </div>
      <div className="room__row">
        {yourSeat < 0 && (
          <button type="button" className="btn btn--primary" disabled={busy} onClick={() => onTake(-1)}>
            {t("vbugroom.22")}
          </button>
        )}
        {yourSeat >= 0 && (
          <button type="button" className="btn btn--ghost" onClick={() => send("leave")}>
            {t("vbugroom.23")}
          </button>
        )}
        {yourSeat >= 0 && (
          <button type="button" className="btn btn--ghost" onClick={() => send("abort")}>
            {t("gameRoom.18")}
          </button>
        )}
      </div>
    </section>
  );
}

// A game of Bughouse: two boards side by side (on a narrow screen one big and one small, switched by a button),
// a pair of players with clocks and a pocket at each, one chat. The server chooses the board of a player by seat.
export default function BughouseRoom({ id }) {
  const t = useT();
  const { user, google, guest } = useAuth();
  const { settings } = useSettings();
  const { snap, status, notice, rev, send } = useGame(id, user?.id, GAME_API);
  const clocks = useBugClocks(snap);
  const narrow = useMedia("(max-width: 860px)");
  const [ui, setUiState] = useState({});
  const [showSummary, setShowSummary] = useState(true);
  const [confirmResign, setConfirmResign] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [activeBoard, setActive] = useState(null);

  const game = snap?.game;
  const yourSeat = game ? game.yourSeat : -1;
  const moves = game?.moves || [];
  const def = getVariant("bughouse");
  const setUi = useCallback((patch) => setUiState((u) => ({ ...u, ...patch })), []);
  useEffect(() => {
    setUiState({});
  }, [moves.length, game?.status]);
  useEffect(() => setConfirmResign(false), [game?.status, moves.length]);

  // Sounds: a move that arrives anywhere, and the end. The first state after opening the page is only remembered.
  const seen = useRef(null);
  useEffect(() => {
    if (!game) return;
    const prev = seen.current;
    seen.current = { moves: moves.length, status: game.status };
    if (!prev || !settings.sound) return;
    if (game.status === "ended" && prev.status !== "ended") playSound("end", settings.soundTheme);
    else if (moves.length > prev.moves) playSound(soundKindForSan(moves[moves.length - 1].san || ""), settings.soundTheme);
  }, [game, moves, settings.sound, settings.soundTheme]);

  const ended = game?.status === "ended";
  useEffect(() => setShowSummary(true), [ended, id]);

  const nextId = game?.rematch?.id;
  useEffect(() => {
    if (nextId && yourSeat >= 0) window.location.hash = `variants/game/${nextId}`;
  }, [nextId, yourSeat]);

  async function take(seat) {
    setBusy(true);
    setError("");
    try {
      if (yourSeat >= 0) {
        if (!send("seat", { seat })) throw new Error(t("vbugroom.25"));
      } else {
        if (!user) await guest();
        await joinGame(id, seat < 0 ? undefined : seat);
      }
    } catch (e) {
      setError(e.message);
    }
    setBusy(false);
  }

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
  const act = (action) => send("move", { action });
  const myBoard = yourSeat >= 0 ? seatBoard(yourSeat) : -1;
  const mySide = yourSeat >= 0 ? seatSide(yourSeat) : "";
  const bottoms = bottomSides(yourSeat, settings.whiteBottom);
  const order = boardOrder(yourSeat);
  const active = activeBoard ?? order[0];

  const info = ended ? describeBugEnd({ result: game.result, termination: game.termination, yourSeat, players: game.players, reasons: def?.reasons }) : null;
  const myTurnNow = playing && yourSeat >= 0 && game.boards[myBoard].turn === mySide;
  let statusLine = "";
  if (game.status === "waiting") statusLine = t("vbugroom.29", { length: game.players.filter(Boolean).length });
  else if (playing) statusLine = yourSeat < 0 ? t("vbugroom.30") : myTurnNow ? t("vbugroom.31", { p: myBoard + 1 }) : t("vbugroom.32", { p: myBoard + 1 });
  else if (game.status === "aborted") statusLine = t("gameRoom.44");
  else if (ended) statusLine = info.subtitle;

  const yourTeam = yourSeat >= 0 ? teamOf(yourSeat) : -1;
  const rematch =
    yourSeat >= 0
      ? { label: rematchLabel(game.rematch, yourSeat), onClick: () => send("rematch") }
      : undefined;
  const chat = chatForRoom(game, yourSeat);
  const allSeated = game.players.every(Boolean);
  const Dnd = ChessboardDnDProvider;
  const firstMoveLeft = game.firstMoveBy ? Math.max(0, Math.ceil((game.firstMoveBy - Date.now()) / 1000)) : 0;

  return (
    <div className={`room vroom bug${narrow ? " bug--narrow" : ""}${game.status === "waiting" ? " bug--waiting" : ""}`} data-variant="bughouse">
      {status === "reconnecting" && <div className="banner banner--error room__net">{t("gameRoom.46")}</div>}
      <Dnd>
        <div className="bug__boards">
          {order.map((b) => (
            <BoardColumn
              key={b}
              game={game}
              board={b}
              yourSeat={yourSeat}
              bottom={bottoms[b]}
              active={b === active}
              mini={narrow && b !== active}
              ui={ui}
              setUi={setUi}
              act={act}
              rev={rev}
              clocks={clocks}
              settings={settings}
              onActivate={() => setActive(b)}
            />
          ))}
          {ended && showSummary && info && (
            <GameSummary
              info={info}
              white={teamNames(game.players, 0)}
              black={teamNames(game.players, 1)}
              you={yourTeam < 0 ? "" : yourTeam === 0 ? "white" : "black"}
              moves={Math.ceil(moves.length / 2)}
              signedIn={!!user}
              newLabel={t("menu.variants")}
              rematch={rematch}
              hint={t("vbugroom.36")}
              onNew={() => (window.location.hash = "variants/bughouse")}
              onClose={() => setShowSummary(false)}
            />
          )}
        </div>
      </Dnd>

      <div className="bug__panel">
        <section className="card room__status">
          <p className="room__state">{statusLine}</p>
          <p className="room__meta">
            {tx(game.variantName)}, {describeTC(game.tc)}
            {game.presence.watching > 0 ? t("gameRoom.50", { watching: game.presence.watching }) : ""}
          </p>
          {yourSeat >= 0 && (
            <p className="room__meta">
              {t("vbugroom.38")} {teamNames(game.players, yourTeam)}. {seatLabel(yourSeat)}.
            </p>
          )}
          {playing && myTurnNow && ui.drop && <p className="room__meta vroom__hint">{t("vcrazy.9")}</p>}
          {notice && <p className="room__notice">{notice}</p>}
          {playing && firstMoveLeft > 0 && moves.length < 4 && <p className="room__meta">{t("vbugroom.40")}</p>}
          {yourSeat < 0 && playing && <p className="room__meta">{t("gameRoom.56")}</p>}
        </section>

        {game.status === "waiting" && (
          <SeatsCard id={id} game={game} yourSeat={yourSeat} send={send} onTake={take} busy={busy} error={error} />
        )}
        {game.status === "waiting" && yourSeat < 0 && !user && google && (
          <a className="btn btn--google btn--block" href={`/api/auth/google/login?next=${encodeURIComponent(`#variants/game/${id}`)}`}>
            <GoogleLogo size={16} /> {t("auth.signInGoogle")}
          </a>
        )}

        {game.status === "aborted" && (
          <section className="card room__status">
            <p className="room__meta">{t("gameRoom.57")}</p>
            <a className="btn btn--primary" href="#variants/bughouse">
              {t("menu.variants")}
            </a>
          </section>
        )}

        {playing && yourSeat >= 0 && (
          <section className="card room__actions">
            {confirmResign ? (
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
              </div>
            )}
            <p className="room__meta">{t("vbugroom.47")}</p>
            {moves.length === 0 && (
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

        <MovesCard moves={moves} />

        {allSeated && (playing || ended) && <Chat game={chat.game} you={chat.you} send={send} />}
      </div>
    </div>
  );
}

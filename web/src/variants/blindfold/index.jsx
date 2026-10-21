// Blindfold chess: the usual rules, but no piece is drawn. The server sends no position while the game is on
// (no pieces, no check; the two squares of the last move stay and are lit on the empty board; with peeking allowed in the lobby the position comes in `extra.board`
// and is shown for three seconds on request), so this file draws an empty board with its coordinates and
// plays by clicks on squares (the legal actions are all it knows) and by text (the server reads "Nf3", "e2e4",
// "O-O" and answers politely). Nothing here may put a piece's name into the DOM (title, aria-label, text) while
// the game is on; the words about the opponent's move are the one thing a player is told, as text.
// Rules and hiding: backend/internal/variants/blindfold.go.
import { useEffect, useState, useSyncExternalStore } from "react";
import Icon from "../../ui/Icon.jsx";
import Segmented from "../../ui/Segmented.jsx";
import {
  PEEK_MS,
  PROMOTION_NAMES,
  boardToShow,
  clickResult,
  lastMoveLine,
  lastMoveToShow,
  moveNumber,
  readPrefs,
  targetsOf,
  writePrefs,
} from "./blindfold.js";
import "./blindfold.css";

// ---------------------------------------------------- the person's settings of the room (kept in the browser)

function storage() {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}
let prefs = readPrefs(storage());
const listeners = new Set();
const getPrefs = () => prefs;
const subscribe = (fn) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};
function setPrefs(patch) {
  prefs = { ...prefs, ...patch };
  writePrefs(storage(), prefs);
  listeners.forEach((fn) => fn());
}
const usePrefs = () => useSyncExternalStore(subscribe, getPrefs);

// ------------------------------------------------------------------------------------- lobby

function PeekOptions({ value, onChange }) {
  const on = !!value?.peek;
  return (
    <div className="blind-opts">
      <p className="blind-opts__title">Подглядывание</p>
      <Segmented
        label="Подглядывание"
        value={on ? "on" : "off"}
        onChange={(v) => onChange({ peek: v === "on" })}
        options={[
          ["off", "Выключено"],
          ["on", "Разрешено"],
        ]}
      />
      <p className="blind-opts__note">
        {on
          ? "У каждого игрока будет кнопка «Подсмотреть позицию на 3 секунды»; зрители видят доску."
          : "Доска пуста всю партию, позицию нельзя посмотреть: только держать в голове (подсвечен лишь последний ход соперника). Позиция откроется после конца партии."}{" "}
        Быстрая игра подбирает соперника с таким же выбором.
      </p>
    </div>
  );
}

// ------------------------------------------------------------------------- the room: above the board

function Top({ game, you, playing }) {
  if (game.status === "waiting") return null;
  const ended = game.status === "ended";
  const moves = game.moves || [];
  const line = lastMoveLine(moves, you, ended);
  let sub;
  if (ended) sub = "Партия окончена: позиция открыта.";
  else if (!playing) sub = "";
  else if (moves.length === 0) sub = you ? "Фигур на доске не видно. Ходите кликами по клеткам или записью хода." : "Игроки ходят вслепую.";
  else sub = moveNumber(moves) ? `Ход ${moveNumber(moves)}` : "";
  return (
    <div className="blind-top" role="status" aria-live="polite">
      <span className="blind-top__icon">
        <Icon name="blindfold" size={20} />
      </span>
      <div className="blind-top__text">
        <p className="blind-top__main">{line || (playing ? "Партия началась" : "")}</p>
        {sub && <p className="blind-top__sub">{sub}</p>}
      </div>
    </div>
  );
}

// -------------------------------------------------------------------- the room: below the board

function Bottom({ game, you, myTurn, ui, setUi, act, notice, view }) {
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const moves = game.moves?.length || 0;
  const peeking = !!ui.peek;

  // a new position: the typed text was played (or is stale)
  useEffect(() => {
    setText("");
    setError("");
  }, [moves]);
  // the server's refusal stays by the field until the text is changed
  useEffect(() => {
    if (notice) setError(notice);
  }, [notice]);
  // the peek ends by itself
  useEffect(() => {
    if (!peeking) return undefined;
    const t = setTimeout(() => setUi({ peek: false }), PEEK_MS);
    return () => clearTimeout(t);
  }, [peeking, setUi]);

  if (!you || game.status !== "playing") return null;
  const canPeek = !!view?.extra?.peek;

  function submit(e) {
    e.preventDefault();
    const t = text.trim();
    if (!t) return setError("Введите ход, например Nf3, e4 или O-O.");
    setError("");
    if (!act(t)) setError("Нет связи с сервером, попробуйте ещё раз.");
  }

  const promo = ui.promo;
  return (
    <div className="blind-bottom">
      {promo && (
        <div className="blind-promo" role="group" aria-label="Во что превратить пешку">
          <span className="blind-promo__lab">Пешка превращается в:</span>
          {promo.letters.map((l) => (
            <button key={l} type="button" className="chip" onClick={() => act(promo.from + promo.to + l)}>
              {PROMOTION_NAMES[l]}
            </button>
          ))}
          <button type="button" className="btn btn--ghost" onClick={() => setUi({ promo: null })}>
            Отмена
          </button>
        </div>
      )}
      <form className="blind-form" onSubmit={submit}>
        <input
          className="field blind-form__input"
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setError("");
          }}
          disabled={!myTurn}
          placeholder={myTurn ? "Ход: Nf3, e4, O-O, e2e4" : "Дождитесь хода соперника"}
          aria-label="Ход текстом"
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          maxLength={16}
        />
        <button type="submit" className="btn btn--primary" disabled={!myTurn}>
          Сделать ход
        </button>
        {canPeek && (
          <button type="button" className="btn blind-form__peek" disabled={peeking} onClick={() => setUi({ peek: true })}>
            <Icon name="board" size={16} /> {peeking ? "Позиция открыта…" : "Подсмотреть позицию на 3 секунды"}
          </button>
        )}
      </form>
      {(error || ui.msg) && (
        <p className="blind-bottom__error" role="alert">
          {error || ui.msg}
        </p>
      )}
    </div>
  );
}

// ------------------------------------------------------------------- the room: card on the right

function Side({ game, view, you, setUi }) {
  const p = usePrefs();
  const ended = game.status === "ended";
  const peek = !!view?.extra?.peek;
  return (
    <section className="card blind-note">
      <h2>
        <Icon name="blindfold" size={18} /> Слепые шахматы
      </h2>
      {ended ? (
        <p className="blind-note__over">Партия окончена: позиция и ходы открыты.</p>
      ) : (
        <>
          <p>Фигур не видно. Нажмите клетку, с которой ходите, потом клетку, куда идёт фигура, или впишите ход в поле под доской.</p>
          <p>Ход соперника подсвечивается на доске двумя клетками (какая фигура, не показано) и пишется текстом над доской. Подсмотреть позицию {peek ? "можно кнопкой под доской" : "нельзя (выключено в этой партии)"}.</p>
          {you && (
            <label className="blind-note__check">
              <input type="checkbox" checked={p.dots} onChange={(e) => setPrefs({ dots: e.target.checked })} />
              <span>Точки допустимых ходов у выбранной клетки</span>
            </label>
          )}
          {you && (
            <label className="blind-note__check">
              <input type="checkbox" checked={p.oppLast} onChange={(e) => {
                  setPrefs({ oppLast: e.target.checked });
                  setUi({}); // the room draws again with the new choice
                }} />
              <span>Подсвечивать последний ход соперника (если включена подсветка хода в настройках доски)</span>
            </label>
          )}
          {!you && <p>Вы наблюдаете за партией{peek ? "" : ": позиция зрителям тоже не видна"}.</p>}
        </>
      )}
    </section>
  );
}

// ------------------------------------------------------------- the room: the card of the moves

// The list of moves is hidden from the players unless they ask (the point is to keep the position in the head);
// a spectator and everyone after the end see it.
function MovesCard({ game, you, moves, children }) {
  const p = usePrefs();
  const over = game.status !== "playing" && game.status !== "waiting";
  const shown = !you || over || p.showMoves;
  return (
    <section className="card room__moves blind-moves">
      <div className="blind-moves__head">
        <h2>Ходы</h2>
        {!!you && !over && (
          <button type="button" className="btn btn--ghost" aria-pressed={p.showMoves} onClick={() => setPrefs({ showMoves: !p.showMoves })}>
            {p.showMoves ? "Скрыть" : "Показать"}
          </button>
        )}
      </div>
      {shown ? children : <p className="blind-moves__off">Лента ходов скрыта: держите позицию в голове. Сыграно полуходов: {moves.length}.</p>}
    </section>
  );
}

export default {
  id: "blindfold",
  name: "Слепые шахматы",
  short: "Обычные правила, но фигур на доске не видно: позицию держите в голове, ходы вводите текстом или кликами по клеткам.",
  rules: [
    "Правила обычные, но на доске нет фигур: видны только клетки и координаты. Последний ход соперника подсвечен двумя клетками (откуда и куда, без фигуры; цвет и включение берутся из настроек доски), шах не показывается.",
    "Ходить можно кликами (нажмите клетку, с которой ходите, и клетку, куда; у выбранной клетки видны точки допустимых ходов) или записью: Nf3, e4, exd5, e8=Q, O-O, e2e4. Если запись непонятна, сайт подскажет.",
    "Ход соперника записан текстом над доской. Лента ходов скрыта (её можно открыть кнопкой в правой колонке): позицию надо держать в голове.",
    "Подглядывание выключено, пока вы не разрешите его в настройках партии: тогда у каждого игрока будет кнопка «Подсмотреть позицию на 3 секунды». После конца партии позиция открывается.",
  ],
  icon: "blindfold",
  order: 30,
  captured: false,
  lobby: { defaults: { peek: false }, Options: PeekOptions },
  PanelTop: Top,
  PanelBottom: Bottom,
  Side,
  MovesCard,

  // the opponent's last move is lit on the empty board (a spectator sees the last move of either side)
  lastMove: (ctx) =>
    lastMoveToShow({
      last: ctx.view?.last,
      moves: ctx.game?.moves,
      you: ctx.you,
      blind: !!ctx.view?.extra?.blind,
      oppLast: getPrefs().oppLast,
    }),

  boardOf: (ctx) => boardToShow({ extra: ctx.view?.extra, you: ctx.you, peeking: !!ctx.ui.peek }),

  // the chosen square and the dots of its moves, drawn without saying what stands there
  boardProps: (ctx) => {
    const out = { extraDots: {}, squareClasses: {} };
    if (ctx.myTurn && ctx.ui.from) {
      out.squareClasses[ctx.ui.from] = "sq-blind-from";
      if (getPrefs().dots) for (const t of targetsOf(ctx.legal, ctx.ui.from)) out.extraDots[t] = "dot";
    }
    return out;
  },

  // Every click on the board is ours while the game is on: there is no piece to take up.
  onSquare: (square, ctx) => {
    if (!ctx.view?.extra?.blind) return false; // the game is over: the board as usual
    if (!ctx.playing || !ctx.myTurn) return true;
    if (ctx.ui.promo) {
      ctx.setUi({ promo: null });
      return true;
    }
    const r = clickResult(ctx.legal, ctx.ui, square);
    if (r.type === "select") ctx.setUi({ from: r.from, msg: "" });
    else if (r.type === "clear") ctx.setUi({ from: null });
    else if (r.type === "move") {
      ctx.setUi({ from: null });
      ctx.act(r.action);
    } else if (r.type === "promote") ctx.setUi({ from: null, promo: { from: r.from, to: r.to, letters: r.letters } });
    else ctx.setUi({ msg: "С этой клетки ходить нечем." });
    return true;
  },

  hint: (ctx) => {
    if (ctx.ui.promo) return "Выберите, во что превратить пешку.";
    if (ctx.ui.from) return "Теперь нажмите клетку, куда идёт фигура.";
    return "Нажмите клетку, с которой ходите, или впишите ход.";
  },
};

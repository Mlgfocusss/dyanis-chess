// Dice chess. A turn is up to three moves. At its start the server throws three dice and each names a kind of piece
// (1 pawn, 2 knight, 3 bishop, 4 rook, 5 queen, 6 king); they are shown together and the side moves with them in any
// order. view.extra = { of, dice: [{ die, state }], left, turns, lost? } (see dice.js). The throw is made by the server
// when the turn begins; the button "Бросить кубики" only reveals it to the person to move (the board is locked until
// then). The reveal is a big animation in the middle of the screen: three dice roll and stop one after another, each
// face a piece; then they land in the strip above the board. The other side sees the same animation when a new throw
// (a new turn counter) appears in its snapshot; a reload or an ordinary refresh does not play it again.
// A die no piece can play is skipped at once and for good. The only actions are piece moves.
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import Icon from "../../ui/Icon.jsx";
import { NAME, describeLost, describeTurn, dieCaption, diceSlots, movableSquares, needsRoll, newThrow, pieceIcon, shakeKinds, sideOfDice } from "./dice.js";
import { useT } from "../../i18n/index.js";
import "./dice.css";

// One die with the piece on its face. state: "free", "used", "skip", "empty" (nothing there yet).
export function PieceDie({ kind, state, side, size = 52, rolling = false, landing = false }) {
  const t = useT();
  return (
    <div
      className={`pdie pdie--${state} pdie--${side}${rolling ? " pdie--rolling" : ""}${landing ? " pdie--landing" : ""}`}
      style={{ width: size, height: size }}
      role="img"
      aria-label={kind ? t("vdice.1", { p: NAME[kind] }) : t("vdice.2")}
    >
      {kind && <Icon name={pieceIcon(kind)} size={Math.round(size * 0.68)} />}
    </div>
  );
}

const STOP_FIRST_MS = 600; // the first die stops
const STOP_STEP_MS = 250; // the next ones follow at this interval
const HOLD_MS = 450; // all three stand still, big
const LAND_MS = 380; // they shrink into the strip

// The big animation over the middle of the screen. dice: the three dice as thrown; onLand: they begin to land
// (the strip shows them); onDone: it is over. Esc or a click on the veil ends it at once.
function RollOverlay({ dice, side, onLand, onDone }) {
  const t = useT();
  const [shown, setShown] = useState(() => shakeKinds());
  const [stopped, setStopped] = useState(0);
  const [leaving, setLeaving] = useState(false);
  const stoppedRef = useRef(0);
  const timers = useRef([]);
  const landed = useRef(false);

  function land() {
    if (landed.current) return;
    landed.current = true;
    setStopped(3);
    setLeaving(true);
    onLand();
    timers.current.push(setTimeout(onDone, LAND_MS));
  }

  useEffect(() => {
    const spin = setInterval(() => {
      const k = shakeKinds();
      setShown((cur) => cur.map((c, i) => (i < stoppedRef.current ? c : k[i])));
    }, 80);
    timers.current.push(spin);
    dice.forEach((d, i) => {
      timers.current.push(
        setTimeout(() => {
          stoppedRef.current = i + 1;
          setStopped(i + 1);
          setShown((cur) => cur.map((c, j) => (j === i ? d.die : c)));
        }, STOP_FIRST_MS + i * STOP_STEP_MS)
      );
    });
    timers.current.push(setTimeout(land, STOP_FIRST_MS + (dice.length - 1) * STOP_STEP_MS + HOLD_MS));
    const onKey = (e) => e.key === "Escape" && land();
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      timers.current.forEach((t) => {
        clearInterval(t);
        clearTimeout(t);
      });
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return createPortal(
    <div className={`dice-veil${leaving ? " dice-veil--leaving" : ""}`} onClick={land} role="presentation">
      <div className="dice-veil__row" aria-live="polite">
        {dice.map((d, i) => {
          const done = i < stopped;
          const kind = done ? d.die : shown[i];
          const state = done ? d.state : "free";
          return (
            <div key={i} className="dice-veil__slot">
              <PieceDie kind={kind} state={state} side={side} size={112} rolling={!done} landing={done && !leaving} />
              <span className="dice-veil__name">{done ? NAME[d.die] : " "}</span>
              <span className="dice-veil__cap">{done ? dieCaption(d.state) : " "}</span>
            </div>
          );
        })}
      </div>
    </div>,
    document.body
  );
}

function DiceBar(ctx) {
  const t = useT();
  const { playing, view, myTurn, ui, setUi, settings } = ctx;
  const ex = view?.extra;
  const [anim, setAnim] = useState(null); // { dice, side, landed } while the big animation runs
  const seen = useRef(undefined); // the turn counter of the throw last shown
  const sawWaiting = useRef(false); // this page was open before the game began: its first throw is really new

  const reduced = () =>
    !!settings?.reduceMotion || (typeof window !== "undefined" && !!window.matchMedia?.("(prefers-reduced-motion: reduce)").matches);

  // A new throw in the snapshot: the other side watches it roll. The first view only remembers the throw (a reload
  // must not play it again), except on a page that was open while the game began: that throw is new.
  useEffect(() => {
    if (!playing) {
      sawWaiting.current = true;
      return;
    }
    if (!ex?.dice) return;
    const first = seen.current === undefined;
    const r = newThrow(seen.current, ex);
    seen.current = r.seen;
    if ((r.fresh || (first && sawWaiting.current)) && !myTurn && !reduced()) setAnim({ dice: ex.dice, side: sideOfDice(view.turn), landed: false });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ex?.turns, playing]);

  if (!playing || !ex || !ex.dice) return null;

  const side = sideOfDice(view.turn);
  const waiting = needsRoll(ex, myTurn, ui.rolled);
  const faces = diceSlots(ex);

  function roll() {
    if (reduced()) {
      setUi({ rolled: true });
      return;
    }
    setAnim({ dice: ex.dice, side, landed: false, mine: true });
  }

  const overlay = anim && (
    <RollOverlay
      key={anim.dice.map((d) => d.die).join("") + ex.turns}
      dice={anim.dice}
      side={anim.side}
      onLand={() => {
        setAnim((a) => a && { ...a, landed: true });
        if (anim.mine) setUi({ rolled: true });
      }}
      onDone={() => setAnim(null)}
    />
  );

  if (waiting && !anim) {
    return (
      <div className="dice-bar dice-bar--roll">
        <button type="button" className="btn btn--primary dice-roll" onClick={roll}>
          <Icon name="dice" size={36} />
          <span className="dice-roll__text">
            <strong>{t("vdice.3")}</strong>
            <span>{t("vdice.4")}</span>
          </span>
        </button>
      </div>
    );
  }

  // while the big dice roll, the strip holds empty places for them to land in
  const hold = anim && !anim.landed;
  return (
    <>
      {overlay}
      <div className="dice-bar" aria-live="polite">
        <div className="dice-bar__dice">
          {faces.map((f, i) => (
            <div key={i} className={`dice-bar__slot dice-bar__slot--${hold || !f ? "empty" : f.state}`}>
              <PieceDie kind={hold ? null : f?.die} state={hold || !f ? "empty" : f.state} side={side} landing={!!anim?.landed} />
              <span className="dice-bar__name">{!hold && f ? NAME[f.die] : ""}</span>
              <span className="dice-bar__cap">{!hold && f ? dieCaption(f.state) : ""}</span>
            </div>
          ))}
        </div>
        <div className="dice-bar__text">
          <span className="dice-bar__now">{hold ? t("vdice.5") : describeTurn(ex, myTurn, view.turn)}</span>
          {ex.lost && !hold && (
            <span className="dice-bar__lost">
              <Icon name="flag" size={14} /> {describeLost(ex.lost)}
            </span>
          )}
        </div>
      </div>
    </>
  );
}

export default {
  id: "dice",
  name: "Dice chess",
  short: "В начале очереди бросаются три кубика и сразу показывают три типа фигур; ходите ими в любом порядке.",
  icon: "dice",
  order: 50,
  rules: [
    "В начале очереди бросаются три кубика. Каждый называет тип фигуры: 1 пешка, 2 конь, 3 слон, 4 ладья, 5 ферзь, 6 король. Все три типа видны сразу.",
    "Ходите выпавшими фигурами в любом порядке, по ходу на каждый кубик. Одинаковые кубики взаимозаменяемы; одной и той же фигурой можно ходить несколько раз, если кубики позволяют.",
    "Если выпавшей фигурой нечем ходить, она сразу пропускается, и пропуск окончательный: даже если после другого хода у неё появится ход, кубик не вернётся. То же после каждого хода: любая оставшаяся фигура, которой нечем ходить, пропускается. Если остались только пропущенные, очередь заканчивается, а если ходить нечем ни одной из трёх с самого начала, она целиком переходит к сопернику.",
    "Шаха и мата нет: король может стоять под боем, а партия кончается, когда его взяли. Рокировка считается ходом короля.",
    "Кубики бросает сервер, соперник видит те же три фигуры. После перезагрузки страницы кубики те же.",
  ],
  reasons: {
    stalemate: ["Нет ходов", ""],
    "fifty-moves": ["50 очередей без взятий и ходов пешками", ""],
  },
  moveSep: " | ",
  PanelTop: DiceBar,
  // the board stays shut until the person to move has pressed the die
  boardLocked: (ctx) => !!ctx.playing && needsRoll(ctx.view?.extra, ctx.myTurn, ctx.ui?.rolled),
  hint: (ctx) => {
    const ex = ctx.view?.extra;
    if (!ex?.dice) return "";
    if (needsRoll(ex, ctx.myTurn, ctx.ui?.rolled)) return "Бросьте кубики.";
    return "Фигуры, которыми можно ходить, подсвечены. Порядок кубиков на ваш выбор.";
  },
  boardProps: (ctx) => {
    if (!ctx.myTurn || needsRoll(ctx.view?.extra, ctx.myTurn, ctx.ui?.rolled)) return {};
    return { squareClasses: movableSquares(ctx.legal) };
  },
};

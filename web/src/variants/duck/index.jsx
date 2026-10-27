// Duck chess. A turn is two actions of one side: a piece move, then the duck put on a free square.
// The server sends view.extra = { step: "move" | "duck", duck: "e5" | "" } and, in the duck step, the
// actions "@e5" (every free square except the one the duck stands on). The duck is the piece "xD".
import Icon from "../../ui/Icon.jsx";
import { duckClick, duckDots, stepOf } from "./duck.js";
import { useT } from "../../i18n/index.js";
import "./duck.css";

// The duck, drawn from design tokens only (so it follows the light theme): an outline pass under a fill pass,
// so that the overlapping shapes show one silhouette.
function DuckShapes({ pass }) {
  const t = useT();
  const cls = pass === "line" ? "duck__line" : "duck__body";
  return (
    <g className={cls}>
      <ellipse cx="46" cy="64" rx="28" ry="19" />
      <circle cx="66" cy="34" r="15" />
      <path d="M22 56 8 44l22 6z" />
    </g>
  );
}

function DuckPiece({ squareWidth }) {
  return (
    <svg className="duck" width={squareWidth} height={squareWidth} viewBox="0 0 100 100" aria-hidden="true">
      <DuckShapes pass="line" />
      <path className="duck__line" d="M78 28l15 6-15 7z" />
      <DuckShapes pass="body" />
      <path className="duck__beak" d="M78 28l15 6-15 7z" />
      <path className="duck__wing" d="M32 62c6-9 20-10 28-2-6 10-22 12-28 2z" />
      <circle className="duck__eye" cx="70" cy="30" r="2.6" />
    </svg>
  );
}

// Module level: a new object would make the board rebuild itself.
const PIECES = { xD: DuckPiece };

// The card in the side column: which of the two actions of the turn is under way, and the confirmation of a
// duck put down (when the person asked for confirmed moves).
function DuckSide(ctx) {
  const t = useT();
  const { playing, view, myTurn, ui, setUi, act } = ctx;
  if (!playing) return null;
  const step = stepOf(ctx);
  const who = view.turn === "white" ? t("gameRoom.sideWhite") : t("gameRoom.sideBlack");
  const pending = myTurn && step === "duck" ? ui.duckPending : null;
  return (
    <section className="card duck-steps" aria-label={t("vduck.3")}>
      <ol className="duck-steps__list">
        <li className={`duck-steps__item ${step === "move" ? "is-now" : "is-done"}`}>
          <span className="duck-steps__no">1</span> {t("vduck.4")}
        </li>
        <li className={`duck-steps__item ${step === "duck" ? "is-now" : ""}`}>
          <span className="duck-steps__no">
            <Icon name="duck" size={14} />
          </span>{" "}
          {t("vduck.5")}
        </li>
      </ol>
      <p className="duck-steps__who">{myTurn ? t("gameRoom.41") : t("vduck.7", { who })}: {step === "move" ? t("vduck.8") : t("vduck.9")}</p>
      {pending && (
        <div className="room__row duck-steps__confirm">
          <button type="button" className="btn btn--primary" onClick={() => act(`@${pending}`)}>
            {t("vduck.10")} {pending}
          </button>
          <button type="button" className="btn btn--ghost" onClick={() => setUi({ duckPending: null })}>
            {t("common.cancel")}
          </button>
        </div>
      )}
    </section>
  );
}

export default {
  id: "duck",
  name: "Утиные шахматы",
  short: "После каждого хода фигурой вы ставите утку на любое свободное поле: она никого не пускает и сама не берётся.",
  icon: "duck",
  order: 40,
  rules: [
    "Ход состоит из двух действий: сначала ход фигурой, затем утка ставится на любое свободное поле (но не на то, где она только что стояла).",
    "Утку нельзя взять, а проходить сквозь неё нельзя: для слона, ладьи, ферзя и пешки она стена. Конь через неё перепрыгивает.",
    "Шаха и мата нет: король может стоять под боем, а партия кончается, когда короля взяли.",
    "Если у вас не осталось ни одного хода фигурой, вы выигрываете.",
    "Рокировка разрешена, даже когда поля под боем, но не через утку и не на неё. Первую утку белые ставят после своего первого хода.",
  ],
  reasons: {
    "no-moves": ["Не осталось ходов", "— у них не осталось ходов, а по правилам это победа"],
  },
  pieces: PIECES,
  hint: (ctx) => (stepOf(ctx) === "duck" ? "Поставьте утку на любое свободное поле: нажмите на клетку." : "Сделайте ход, затем поставьте утку."),
  Side: DuckSide,
  boardProps: (ctx) => {
    if (!ctx.myTurn || stepOf(ctx) !== "duck") return {};
    return { extraDots: duckDots(ctx.legal), hint: ctx.ui.duckPending || undefined };
  },
  onSquare: (square, ctx) => {
    const r = duckClick(square, ctx);
    if (r.pending) ctx.setUi({ duckPending: r.pending });
    if (r.act) ctx.act(r.act);
    return r.handled;
  },
};

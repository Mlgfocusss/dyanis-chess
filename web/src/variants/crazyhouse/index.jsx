// Crazyhouse (as on Lichess): a piece taken goes into the taker's pocket and may be dropped on any empty square
// instead of a move. The server (variants/crazyhouse.go) offers the drops as legal actions "N@f3"; here they are
// chosen by a click on the pocket's piece and then on a square, or by dragging the piece onto the board.
import Pocket from "../pockets/Pocket.jsx";
import { droppableKinds, dropClick, dropDots, kindOfCode, pocketCounts, promotedClasses, sideOfCode, dropAction } from "../pockets/pockets.js";
import { useT } from "../../i18n/index.js";
import "./crazyhouse.css";

// the pocket at one end of the board: the side at that end plays from the bottom of the screen if it is the viewer's
function PocketBar({ end, ...ctx }) {
  const t = useT();
  const side = end === "top" ? (ctx.orientation === "white" ? "black" : "white") : ctx.orientation;
  const mine = ctx.you === side;
  const counts = pocketCounts(ctx.view?.extra, side);
  const usable = mine && ctx.myTurn;
  return (
    <Pocket
      side={side}
      counts={counts}
      interactive={usable}
      droppable={usable ? droppableKinds(ctx.legal) : undefined}
      selected={mine ? ctx.ui.drop || null : null}
      onSelect={(k) => ctx.setUi({ drop: k })}
      label={t("vcrazy.1")}
      note={mine ? t("vcrazy.2") : undefined}
    />
  );
}

const PanelTop = (ctx) => <PocketBar end="top" {...ctx} />;
const PanelBottom = (ctx) => <PocketBar end="bottom" {...ctx} />;

export default {
  id: "crazyhouse",
  name: "Crazyhouse",
  short: "Взятые фигуры уходят в ваш резерв, и вместо хода их можно выставить обратно на любое свободное поле.",
  icon: "crazyhouse",
  order: 60,
  rules: [
    "Съеденная вами фигура попадает в ваш резерв (превращённая пешка при взятии становится пешкой).",
    "Вместо хода можно выставить фигуру из резерва на любое свободное поле; пешку нельзя ставить на первую и восьмую горизонтали.",
    "Выставленная фигура может закрыть шах или поставить мат. Рокировка и взятие на проходе обычные.",
    "Пат — ничья. Недостатка материала нет: резерв всегда может вернуть фигуры, поэтому упавший флажок всегда проигрывает.",
    "Нажмите на фигуру в своём резерве и на подсвеченное поле или перетащите фигуру на доску. Esc отменяет выбор.",
  ],
  captured: false, // material is not counted: taken pieces come back
  PanelTop,
  PanelBottom,
  // a click on a square while a piece of the pocket is chosen drops it there
  onSquare(square, ctx) {
    const kind = ctx.ui.drop;
    if (!kind) return false;
    const action = ctx.myTurn ? dropClick(ctx.legal, kind, square) : null;
    ctx.setUi({ drop: null });
    if (!action) return false; // not a square for the drop: the board treats the click as its own (selects a piece)
    ctx.act(action);
    return true;
  },
  // a piece dragged from the pocket onto a square of the board
  onSpareDrop(piece, square, ctx) {
    if (!ctx.myTurn || sideOfCode(piece) !== ctx.you) return;
    const action = dropAction(kindOfCode(piece), square);
    if (ctx.legal.includes(action)) ctx.act(action);
    else ctx.setUi({ drop: null });
  },
  boardProps(ctx) {
    return {
      extraDots: ctx.myTurn && ctx.ui.drop ? dropDots(ctx.legal, ctx.ui.drop) : undefined,
      squareClasses: promotedClasses(ctx.view?.extra),
    };
  },
  hint(ctx) {
    if (ctx.ui.drop) return `Фигура из резерва выбрана: нажмите на подсвеченное поле (Esc — отмена).`;
    const counts = pocketCounts(ctx.view?.extra, ctx.you);
    return Object.values(counts).some((n) => n > 0) ? "В резерве есть фигуры: можно сходить или выставить одну из них." : "";
  },
};

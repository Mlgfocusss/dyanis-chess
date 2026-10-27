import { useEffect, useRef, useState } from "react";
import { SparePiece } from "react-chessboard";
import { useSettings } from "../../settings/SettingsContext.jsx";
import { getPieceSet } from "../../themes/index.js";
import { POCKET_ORDER, pocketTotal } from "./pockets.js";
import { tx, useT } from "../../i18n/index.js";
import "./pockets.css";

const NAMES = { Q: "ферзь", R: "ладья", B: "слон", N: "конь", P: "пешка" };

function useSlotSize() {
  const [size, setSize] = useState(() => (typeof window !== "undefined" && window.innerWidth < 560 ? 34 : 44));
  useEffect(() => {
    const on = () => setSize(window.innerWidth < 560 ? 34 : 44);
    window.addEventListener("resize", on);
    return () => window.removeEventListener("resize", on);
  }, []);
  return size;
}

// The pieces a player holds in hand (Crazyhouse, Bughouse): one slot per kind with the count. A pocket the
// viewer plays from (`interactive`) is chosen by a click (then a square of the board) or its piece is dragged
// onto the board (react-chessboard's SparePiece; the room puts board and pocket in one drag-and-drop provider).
//   side        "white" | "black": the colour of the pieces
//   counts      { Q: 0, R: 1, ... } (pockets.js pocketCounts)
//   interactive the viewer may use it now (their pocket, their turn)
//   droppable   Set of the kinds that have a legal drop now
//   selected    the chosen kind or null;  onSelect(kind | null)
//   boardId     the id of the board the pieces are dragged to (react-chessboard draws the dragged piece only for its own board)
//   label       text before the slots ("Резерв"); note: text after them (whose pocket it is)
export default function Pocket({ side, counts, interactive = false, droppable, selected = null, onSelect, boardId = "DyanisBoard", label, note }) {
  const t = useT();
  const { settings } = useSettings();
  const set = getPieceSet(settings.pieceSet);
  const size = useSlotSize();
  const wasSelected = useRef(false);

  // Escape puts a chosen piece back
  useEffect(() => {
    if (!selected) return undefined;
    const onKey = (e) => {
      if (e.key === "Escape") onSelect?.(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selected, onSelect]);

  const colour = side === "white" ? "w" : "b";
  const total = pocketTotal(counts);
  return (
    <div className={`pocket pocket--${side}${interactive ? " pocket--mine" : ""}`} role="group" aria-label={t("pocket.title", { side: side === "white" ? t("pocket.white") : t("pocket.black") })} data-pocket={side}>
      {(label ?? t("vcrazy.1")) && <span className="pocket__label">{label ?? t("vcrazy.1")}</span>}
      <div className="pocket__slots">
        {POCKET_ORDER.map((k) => {
          const n = counts[k] || 0;
          const code = colour + k;
          const usable = interactive && n > 0 && (!droppable || droppable.has(k));
          const picture = !set.builtin ? ({ squareWidth }) => <img src={set.files[code]} alt="" width={squareWidth} height={squareWidth} draggable={false} /> : undefined;
          return (
            <div
              key={k}
              className={["pocket__slot", n === 0 ? "pocket__slot--zero" : "", usable ? "pocket__slot--use" : "", selected === k ? "pocket__slot--sel" : ""].filter(Boolean).join(" ")}
              style={{ width: size, height: size }}
              data-kind={k}
              data-count={n}
              role={usable ? "button" : undefined}
              tabIndex={usable ? 0 : undefined}
              aria-pressed={usable ? selected === k : undefined}
              aria-label={`${tx(NAMES[k])}: ${n}`}
              title={`${tx(NAMES[k])}: ${n}`}
              onPointerDown={() => {
                if (!usable) return;
                wasSelected.current = selected === k;
                onSelect?.(k); // chosen at once, so the squares of the drop show while the piece is dragged
              }}
              onClick={() => {
                if (usable && wasSelected.current) onSelect?.(null); // a second click puts it back
                wasSelected.current = false;
              }}
              onKeyDown={(e) => {
                if (usable && (e.key === "Enter" || e.key === " ")) {
                  e.preventDefault();
                  onSelect?.(selected === k ? null : k);
                }
              }}
            >
              {/* the piece of an opponent's pocket (or an empty slot) is a picture only: it cannot be dragged */}
              <div className="pocket__piece" style={usable ? undefined : { pointerEvents: "none" }}>
                <SparePiece piece={code} width={size - 4} dndId={boardId} customPieceJSX={picture} />
              </div>
              {n > 1 && <span className="pocket__n">{n}</span>}
            </div>
          );
        })}
      </div>
      {total === 0 && <span className="pocket__none">{t("vpocket.8")}</span>}
      {note && <span className="pocket__note">{note}</span>}
    </div>
  );
}

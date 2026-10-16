import { SparePiece } from "react-chessboard";
import "./promotion.css";

const NAMES = { q: "Ферзь", n: "Конь", r: "Ладья", b: "Слон" };

// The choice of the piece a pawn becomes: a column of squares that opens on the square the pawn
// moves to and runs inwards. Drawn inside that square (see ThemedBoard.jsx), so it needs
// no knowledge of where the board is. letters: "q" | "n" | "r" | "b" in the order offered.
export default function PromotionPicker({ letters, side, size, up, set, onPick }) {
  return (
    <div className={`promo${up ? " promo--up" : ""}`} role="group" aria-label="Во что превратить пешку">
      {letters.map((l) => {
        const code = side + l.toUpperCase();
        return (
          <button
            key={l}
            type="button"
            className="promo__btn"
            title={NAMES[l]}
            aria-label={NAMES[l]}
            // the board under the picker would take the click as a click on a square
            onClick={(e) => {
              e.stopPropagation();
              onPick(l);
            }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            {set.builtin ? (
              <SparePiece piece={code} width={size} dndId={`promo-${code}`} />
            ) : (
              <img src={set.files[code]} alt="" width={size} height={size} draggable={false} />
            )}
          </button>
        );
      })}
    </div>
  );
}

import { arrowShape } from "../review/arrows.js";

// The engine's arrows over a board: a transparent layer the size of the board, in units of one square
// (a viewBox of 8 x 8), that lets every click through. arrows: [{from, to, kind, width, opacity}] from
// review/arrows.js, drawn in the order given (the last is on top); the colour of a kind is a token in
// review.css (.earrow--best and so on). An arrow keeps its element while it stays (the key is its move
// and its kind), so a new position or the arrival of the lines does not flash.
export default function ArrowLayer({ arrows, orientation }) {
  return (
    <svg className="earrows" viewBox="0 0 8 8" aria-hidden="true" focusable="false">
      {arrows.map((a) => {
        const { shaft, head } = arrowShape(a.from, a.to, orientation, a.width);
        return (
          <g key={`${a.uci}-${a.kind}`} className={`earrow earrow--${a.kind}`} style={{ opacity: a.opacity }}>
            <polyline points={shaft.map((p) => p.map((v) => v.toFixed(3)).join(",")).join(" ")} strokeWidth={a.width} className="earrow__shaft" />
            <polygon points={head.map((p) => p.map((v) => v.toFixed(3)).join(",")).join(" ")} className="earrow__head" />
          </g>
        );
      })}
    </svg>
  );
}

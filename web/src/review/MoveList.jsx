import { useEffect, useRef } from "react";
import ClassBadge from "./ClassBadge.jsx";

function Cell({ move, active, onSelect }) {
  const ref = useRef(null);
  useEffect(() => {
    if (active) ref.current?.scrollIntoView({ block: "nearest" });
  }, [active]);
  if (!move) return <span className="move-cell move-cell--empty" />;
  return (
    <button
      ref={ref}
      type="button"
      className={`move-cell${active ? " move-cell--active" : ""}`}
      onClick={() => onSelect(move.ply)}
    >
      <ClassBadge cls={move.class} />
      <span className="move-cell__san">{move.san}</span>
    </button>
  );
}

// Moves in rows of one full move each. The first row may start with
// Black (a game set up from a FEN).
export default function MoveList({ moves, ply, onSelect }) {
  const rows = [];
  let i = 0;
  if (moves.length && moves[0].color === "black") {
    rows.push({ no: Math.ceil(moves[0].ply / 2), white: null, black: moves[0] });
    i = 1;
  }
  for (; i < moves.length; i += 2) {
    rows.push({ no: Math.ceil(moves[i].ply / 2), white: moves[i], black: moves[i + 1] || null });
  }
  return (
    <div className="move-list card">
      {rows.map((r) => (
        <div className="move-row" key={r.no}>
          <span className="move-row__no">{r.no}.</span>
          <Cell move={r.white} active={r.white?.ply === ply} onSelect={onSelect} />
          <Cell move={r.black} active={r.black?.ply === ply} onSelect={onSelect} />
        </div>
      ))}
    </div>
  );
}

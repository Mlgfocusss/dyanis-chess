import { useEffect, useRef } from "react";
import ClassBadge from "./ClassBadge.jsx";
import MoveTime from "../online/MoveTime.jsx";
import { nextScrollTop } from "../ui/scroll.js";
import { useSettings } from "../settings/SettingsContext.jsx";

// The list scrolls by itself to keep the active move in view, and only the list: the panel around it
// (with the lines and the coach above) must not be moved, which scrollWithin would do when the list is short.
function keepInView(el, box) {
  if (!el || !box || box.scrollHeight <= box.clientHeight) return;
  const b = box.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  box.scrollTop = nextScrollTop({ viewTop: b.top, viewHeight: box.clientHeight, itemTop: r.top, itemHeight: r.height, scrollTop: box.scrollTop });
}

function Cell({ move, active, onSelect, boxRef }) {
  const ref = useRef(null);
  const { settings } = useSettings();
  useEffect(() => {
    if (active) keepInView(ref.current, boxRef.current); // only the list, never the panel or the page
  }, [active]);
  if (!move) return <span className="move-cell move-cell--empty" />;
  return (
    <button
      ref={ref}
      type="button"
      className={`move-cell${active ? " move-cell--active" : ""}`}
      onClick={() => onSelect(move.ply)}
    >
      {settings.moveIcons && <ClassBadge cls={move.class} />}
      <span className="move-cell__san">{move.san}</span>
      <MoveTime ms={move.timeMs} />
    </button>
  );
}

// Moves in rows of one full move each. The first row may start with
// Black (a game set up from a FEN).
export default function MoveList({ moves, ply, onSelect }) {
  const boxRef = useRef(null);
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
    <div className="move-list card" ref={boxRef}>
      {rows.map((r) => (
        <div className="move-row" key={r.no}>
          <span className="move-row__no">{r.no}.</span>
          <Cell move={r.white} active={r.white?.ply === ply} onSelect={onSelect} boxRef={boxRef} />
          <Cell move={r.black} active={r.black?.ply === ply} onSelect={onSelect} boxRef={boxRef} />
        </div>
      ))}
    </div>
  );
}

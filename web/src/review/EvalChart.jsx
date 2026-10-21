import { useRef, useState } from "react";
import { CLASSES } from "./classes.js";

// Win-probability curve of the game. The light area is White's share: the higher the curve, the better
// for White. Click or drag to jump to a ply, point at it to see which move it is. The turning points
// (marks: [{ply, cls}]) are dots in the colour of their class on the curve, the move on the board is a
// larger ring with a line through it. The dots are HTML over the picture so the stretched drawing does
// not squash them. labelOf(ply) is the text of the tooltip.
export default function EvalChart({ evals, ply, marks = [], onSelect, labelOf }) {
  const ref = useRef(null);
  const [hover, setHover] = useState(null);
  const n = evals.length - 1; // plies; evals has n+1 entries
  const H = 100;
  const x = (i) => (n === 0 ? 0 : (i / n) * 1000);
  const pct = (i) => (n === 0 ? 0 : (i / n) * 100);
  const top = (i) => 100 - Math.max(0, Math.min(100, evals[i].winPct));

  const line = evals.map((e, i) => `${x(i).toFixed(1)},${(H - e.winPct).toFixed(1)}`).join(" ");
  const area = `0,${H} ${line} 1000,${H}`;

  function plyAt(clientX) {
    const r = ref.current.getBoundingClientRect();
    const f = Math.min(1, Math.max(0, (clientX - r.left) / r.width));
    return Math.round(f * n);
  }

  function onPointerDown(e) {
    e.currentTarget.setPointerCapture(e.pointerId);
    onSelect(plyAt(e.clientX));
  }
  function onPointerMove(e) {
    const p = plyAt(e.clientX);
    setHover(p);
    if (e.buttons & 1) onSelect(p);
  }

  const now = Math.max(0, Math.min(n, ply));
  const tip = hover !== null && labelOf ? labelOf(hover) : "";
  return (
    <div
      ref={ref}
      className="eval-chart"
      role="img"
      aria-label="График оценки партии"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerLeave={() => setHover(null)}
    >
      <svg className="eval-chart__svg" viewBox={`0 0 1000 ${H}`} preserveAspectRatio="none" aria-hidden="true">
        <rect x="0" y="0" width="1000" height={H} className="eval-chart__bg" />
        <polygon points={area} className="eval-chart__white" />
        <line x1="0" x2="1000" y1="50" y2="50" className="eval-chart__mid" vectorEffect="non-scaling-stroke" />
        <polyline points={line} className="eval-chart__line" vectorEffect="non-scaling-stroke" />
        {hover !== null && <line x1={x(hover)} x2={x(hover)} y1="0" y2={H} className="eval-chart__hover" vectorEffect="non-scaling-stroke" />}
        <line x1={x(now)} x2={x(now)} y1="0" y2={H} className="eval-chart__cursor" vectorEffect="non-scaling-stroke" />
      </svg>
      {marks.map((m) =>
        m.ply <= n ? <span key={m.ply} className="eval-chart__mark" style={{ left: `${pct(m.ply)}%`, top: `${top(m.ply)}%`, background: CLASSES[m.cls]?.color }} /> : null
      )}
      <span className="eval-chart__now" style={{ left: `${pct(now)}%`, top: `${top(now)}%` }} />
      {tip && (
        <span className="eval-chart__tip" style={{ left: `${Math.min(88, Math.max(12, pct(hover)))}%` }}>
          {tip}
        </span>
      )}
    </div>
  );
}

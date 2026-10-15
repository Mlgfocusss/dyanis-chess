import { useRef } from "react";

// Win-probability curve of the game. The white area is White's share:
// the higher the curve, the better for White. Click or drag to jump to
// a ply. Red ticks mark the key moments.
export default function EvalChart({ evals, ply, keyPlies, onSelect }) {
  const ref = useRef(null);
  const n = evals.length - 1; // plies; evals has n+1 entries
  const H = 100;
  const x = (i) => (n === 0 ? 0 : (i / n) * 1000);
  const y = (w) => H - w; // winPct is 0..100

  const line = evals.map((e, i) => `${x(i).toFixed(1)},${y(e.winPct).toFixed(1)}`).join(" ");
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
    if (e.buttons & 1) onSelect(plyAt(e.clientX));
  }

  return (
    <svg
      ref={ref}
      className="eval-chart"
      viewBox={`0 0 1000 ${H}`}
      preserveAspectRatio="none"
      role="img"
      aria-label="График оценки партии"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
    >
      <rect x="0" y="0" width="1000" height={H} className="eval-chart__bg" />
      <polygon points={area} className="eval-chart__white" />
      <line x1="0" x2="1000" y1="50" y2="50" className="eval-chart__mid" vectorEffect="non-scaling-stroke" />
      {keyPlies.map((p) => (
        <line
          key={p}
          x1={x(p)}
          x2={x(p)}
          y1="0"
          y2={H}
          className="eval-chart__key"
          vectorEffect="non-scaling-stroke"
        />
      ))}
      <polyline points={line} className="eval-chart__line" vectorEffect="non-scaling-stroke" />
      <line x1={x(ply)} x2={x(ply)} y1="0" y2={H} className="eval-chart__cursor" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

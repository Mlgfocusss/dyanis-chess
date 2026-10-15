import { useMemo, useState } from "react";
import ThemedBoard from "./ThemedBoard.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";

// state: stateResponse from dyanisEngine.getState()/makeMove()/etc.
// onPlayerMove(uci) -> moveResponse; called for both drag-drop and
// click-click attempts once a legal from/to pair is picked. How pieces may
// be moved (drag, click or both), the last-move highlight and everything
// about the board's look come from the user's settings.
export default function Board({ state, orientation, disabled, lastMove, onPlayerMove }) {
  const { settings } = useSettings();
  const [selectedSquare, setSelectedSquare] = useState(null);

  const canDrag = settings.moveMethod !== "click";
  const canClick = settings.moveMethod !== "drag";

  const movesBySquare = useMemo(() => {
    const map = new Map();
    for (const m of state.legalMoves) {
      const from = m.uci.slice(0, 2);
      if (!map.has(from)) map.set(from, []);
      map.get(from).push(m);
    }
    return map;
  }, [state.legalMoves]);

  const legalTargetSquares = useMemo(() => {
    if (!selectedSquare) return new Set();
    return new Set((movesBySquare.get(selectedSquare) || []).map((m) => m.uci.slice(2, 4)));
  }, [selectedSquare, movesBySquare]);

  // Plays from->to if it's a legal move. When more than one candidate
  // matches (only happens for promotion), defaults to queen — same
  // convention the engine's own ParseSAN uses server-side for an
  // unspecified promotion, so behavior matches the CLI. A proper
  // promotion-piece picker is a reasonable follow-up, not blocking.
  function attemptMove(from, to) {
    const candidates = (movesBySquare.get(from) || []).filter((m) => m.uci.slice(2, 4) === to);
    if (candidates.length === 0) return false;
    const choice = candidates.find((m) => m.uci.endsWith("q")) || candidates[0];
    const result = onPlayerMove(choice.uci);
    return result?.ok ?? false;
  }

  function handleSquareClick(square) {
    if (disabled || !canClick) return;
    if (selectedSquare === square) {
      setSelectedSquare(null);
      return;
    }
    if (selectedSquare && legalTargetSquares.has(square)) {
      const moved = attemptMove(selectedSquare, square);
      setSelectedSquare(null);
      if (moved) return;
    }
    setSelectedSquare(movesBySquare.has(square) ? square : null);
  }

  function handlePieceDrop(sourceSquare, targetSquare) {
    if (disabled || !canDrag) return false;
    const moved = attemptMove(sourceSquare, targetSquare);
    setSelectedSquare(null);
    return moved;
  }

  const squareStyles = useMemo(() => {
    const styles = {};
    if (lastMove && settings.highlightLastMove) {
      styles[lastMove.from] = { backgroundColor: "rgba(250, 204, 21, 0.45)" };
      styles[lastMove.to] = { backgroundColor: "rgba(250, 204, 21, 0.45)" };
    }
    if (selectedSquare) {
      styles[selectedSquare] = { ...(styles[selectedSquare] || {}), backgroundColor: "rgba(250, 204, 21, 0.6)" };
    }
    for (const sq of legalTargetSquares) {
      styles[sq] = { ...(styles[sq] || {}), boxShadow: "inset 0 0 0 4px rgba(52, 211, 153, 0.8)" };
    }
    return styles;
  }, [lastMove, selectedSquare, legalTargetSquares, settings.highlightLastMove]);

  return (
    <div className="board-wrap">
      <ThemedBoard
        id="DyanisBoard"
        position={state.fen}
        orientation={orientation}
        onPieceDrop={handlePieceDrop}
        onSquareClick={handleSquareClick}
        isDraggablePiece={({ sourceSquare }) => !disabled && canDrag && movesBySquare.has(sourceSquare)}
        customSquareStyles={squareStyles}
        autoPromoteToQueen
      />
    </div>
  );
}

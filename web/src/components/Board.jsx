import { useEffect, useMemo, useState } from "react";
import ThemedBoard from "./ThemedBoard.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";

// The pieces of a FEN: { "e4": "P", ... } (white upper case, black lower).
function piecesOf(fen) {
  const out = {};
  const rows = (fen || "").split(" ")[0].split("/");
  rows.forEach((row, i) => {
    let file = 0;
    for (const c of row) {
      if (c >= "1" && c <= "8") file += Number(c);
      else out["abcdefgh"[file++] + (8 - i)] = c;
    }
  });
  return out;
}

// state: stateResponse from dyanisEngine.getState()/makeMove()/etc.
// onPlayerMove(uci) -> moveResponse; called for both drag-drop and
// click-click attempts once a legal from/to pair is picked. How pieces may
// be moved (drag, click or both), the last-move highlight and everything
// about the board's look come from the user's settings. hint: a square to
// mark (puzzle hints); badge: {square, cls} for a move-quality badge on a square;
// arrows: [[from, to, colour]]; lastMoveColor replaces the highlight colour.
export default function Board({ state, orientation, disabled, lastMove, lastMoveColor, hint, badge, arrows, onPlayerMove }) {
  const { settings } = useSettings();
  const [selectedSquare, setSelectedSquare] = useState(null);
  // a new position (a move was made, or the review stepped) drops the selection
  useEffect(() => setSelectedSquare(null), [state.fen]);

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

  // Picking a piece up shows where it can go, as a click would.
  function handleDragBegin(piece, square) {
    if (!disabled && canDrag) setSelectedSquare(movesBySquare.has(square) ? square : null);
  }

  function handlePieceDrop(sourceSquare, targetSquare) {
    if (disabled || !canDrag) return false;
    const moved = attemptMove(sourceSquare, targetSquare);
    setSelectedSquare(null);
    return moved;
  }

  // Where the selected piece can go: a dot on an empty square, a ring on a
  // square with a piece to take (or the en passant square).
  const dots = useMemo(() => {
    if (!selectedSquare) return {};
    const pieces = piecesOf(state.fen);
    const mover = pieces[selectedSquare];
    const out = {};
    for (const sq of legalTargetSquares) {
      const ep = !pieces[sq] && (mover === "P" || mover === "p") && sq[0] !== selectedSquare[0];
      out[sq] = pieces[sq] || ep ? "ring" : "dot";
    }
    return out;
  }, [selectedSquare, legalTargetSquares, state.fen]);

  const squareStyles = useMemo(() => {
    const styles = {};
    if (lastMove && settings.highlightLastMove) {
      const color = lastMoveColor || "rgba(250, 204, 21, 0.45)";
      styles[lastMove.from] = { backgroundColor: color };
      styles[lastMove.to] = { backgroundColor: color };
    }
    if (hint) {
      styles[hint] = { ...(styles[hint] || {}), boxShadow: "inset 0 0 0 4px rgba(96, 165, 250, 0.9)" };
    }
    if (selectedSquare) {
      styles[selectedSquare] = { ...(styles[selectedSquare] || {}), backgroundColor: "rgba(250, 204, 21, 0.6)" };
    }
    return styles;
  }, [lastMove, lastMoveColor, hint, selectedSquare, settings.highlightLastMove]);

  return (
    <div className="board-wrap">
      <ThemedBoard
        id="DyanisBoard"
        position={state.fen}
        orientation={orientation}
        onPieceDrop={handlePieceDrop}
        onPieceDragBegin={handleDragBegin}
        onSquareClick={handleSquareClick}
        isDraggablePiece={({ sourceSquare }) => !disabled && canDrag && movesBySquare.has(sourceSquare)}
        customSquareStyles={squareStyles}
        dots={dots}
        {...(badge !== undefined ? { badge } : {})}
        {...(arrows ? { customArrows: arrows } : {})}
        autoPromoteToQueen
      />
    </div>
  );
}

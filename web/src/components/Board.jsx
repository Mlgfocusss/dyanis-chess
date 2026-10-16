import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ThemedBoard from "./ThemedBoard.jsx";
import { useSettings } from "../settings/SettingsContext.jsx";
import { piecesOf, premoveList, previewPlacement, queuedPosition, resolveMove, targetSquares } from "./boardrules.js";
import { pieceSide, promotionLetters } from "./promotion.js";
import Icon from "../ui/Icon.jsx";

// state: stateResponse from dyanisEngine.getState()/makeMove()/etc.
// onPlayerMove(uci) -> moveResponse; called for both drag-drop and
// click-click attempts once a legal from/to pair is picked. How pieces may
// be moved (drag, click or both), the last-move highlight, the dots, the way of
// castling, the automatic queen and everything about the board's look come
// from the user's settings. hint: a square to
// mark (puzzle hints); badge: {square, cls} for a move-quality badge on a square;
// arrows: [[from, to, colour]]; lastMoveColor replaces the highlight colour.
// confirmMove: a move waits for a press on "confirm" before onPlayerMove gets it.
// premove: { enabled, side ("w" | "b"), queue ([uci]), onSet(uci), onClear() }: while the board is
// disabled (the opponent is to move) the person may still choose moves in advance, one after another;
// each is chosen on the position the earlier ones would leave. onSet adds to the queue, onClear empties it.
export default function Board({ state, orientation, disabled, lastMove, lastMoveColor, hint, badge, arrows, onPlayerMove, confirmMove = false, premove }) {
  const { settings } = useSettings();
  const [selectedSquare, setSelectedSquare] = useState(null);
  // A pawn that reached the last rank waits for the choice of the piece: { from, to, letters }.
  const [promoting, setPromoting] = useState(null);
  // A move waiting for confirmation: { uci, from, to }.
  const [pending, setPending] = useState(null);
  // a new position (a move was made, or the review stepped) drops the selection
  useEffect(() => {
    setSelectedSquare(null);
    setPromoting(null);
    setPending(null);
  }, [state.fen]);

  const premoveActive = !!premove?.enabled && !!disabled;
  const locked = (disabled && !premoveActive) || !!pending;
  useEffect(() => {
    if (locked) setPromoting(null);
  }, [locked]);

  // Alt held while moving asks for the choice of the piece for a promotion even with "always a queen" on.
  const alt = useRef(false);
  useEffect(() => {
    const set = (e) => {
      alt.current = e.altKey;
    };
    const off = () => {
      alt.current = false;
    };
    window.addEventListener("keydown", set);
    window.addEventListener("keyup", set);
    window.addEventListener("blur", off);
    return () => {
      window.removeEventListener("keydown", set);
      window.removeEventListener("keyup", set);
      window.removeEventListener("blur", off);
    };
  }, []);

  // Escape takes back a choice of a piece, a move waiting for confirmation, or a premove
  const queue = premove?.queue;
  const queued = queue?.length || 0;
  useEffect(() => {
    if (!promoting && !pending && !queued) return undefined;
    const onKey = (e) => {
      if (e.key !== "Escape") return;
      if (promoting) setPromoting(null);
      else if (pending) setPending(null);
      else premove?.onClear?.();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [promoting, pending, queued]); // eslint-disable-line react-hooks/exhaustive-deps

  const canDrag = settings.moveMethod !== "click";
  const canClick = settings.moveMethod !== "drag";

  // The position the person chooses on: the real one, or, with premoves queued, the one they would leave.
  const shownFen = useMemo(() => (premoveActive ? queuedPosition(state.fen, queue) : state.fen), [premoveActive, state.fen, queue]);
  // the moves the person may make now: the legal ones, or, for a premove, those of the geometry
  const moves = useMemo(() => (premoveActive ? premoveList(shownFen, premove.side) : state.legalMoves), [premoveActive, premove?.side, shownFen, state.legalMoves]);

  const movesBySquare = useMemo(() => {
    const map = new Map();
    for (const m of moves) {
      const from = m.uci.slice(0, 2);
      if (!map.has(from)) map.set(from, []);
      map.get(from).push(m);
    }
    return map;
  }, [moves]);

  const legalTargetSquares = useMemo(() => {
    if (!selectedSquare) return new Set();
    return targetSquares(moves, shownFen, selectedSquare, settings.castling);
  }, [selectedSquare, moves, shownFen, settings.castling]);

  // The library keeps the drag rule of a piece from the render that put it there, so a rule written
  // as a closure goes stale when the legal moves arrive after the position (the opening builder asks
  // the server for them): the pieces stay undraggable. This one reads what is current.
  const live = useRef(null);
  live.current = { locked, canDrag, movesBySquare };
  const isDraggable = useCallback(({ sourceSquare }) => {
    const l = live.current;
    return !l.locked && l.canDrag && l.movesBySquare.has(sourceSquare);
  }, []);

  // The move is made: sent, or kept as a premove, or kept until it is confirmed. true when the
  // board may show the piece where it was dropped.
  function submit(uci) {
    if (premoveActive) {
      premove.onSet(uci);
      return false; // the piece goes back; the premove is drawn as marked squares and the moved piece
    }
    if (confirmMove) {
      setPending({ uci, from: uci.slice(0, 2), to: uci.slice(2, 4) });
      return false; // the preview below shows the move
    }
    const result = onPlayerMove(uci);
    return result?.ok ?? false;
  }

  // Plays from->to if it's a legal move. A promotion has several candidates
  // (queen, knight, rook, bishop): the move waits for the person's choice,
  // and the board answers false (a dragged pawn goes back until then).
  function attemptMove(from, to) {
    const uci = resolveMove(moves, shownFen, from, to, settings.castling);
    if (!uci) return false;
    const letters = promotionLetters(moves, from, to);
    if (letters) {
      if (premoveActive || (settings.alwaysQueen && !alt.current && letters.includes("q"))) return submit(from + to + "q");
      setPromoting({ from, to, letters });
      return false;
    }
    return submit(uci);
  }

  function choosePromotion(letter) {
    const { from, to } = promoting;
    setPromoting(null);
    setSelectedSquare(null);
    submit(from + to + letter);
  }

  function handleSquareClick(square) {
    if (promoting) {
      setPromoting(null); // a click anywhere else takes the choice back
      if (square === promoting.from) {
        setSelectedSquare(null);
        return;
      }
    }
    if (locked || !canClick) return;
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
    if (!locked && canDrag) setSelectedSquare(movesBySquare.has(square) ? square : null);
  }

  function handlePieceDrop(sourceSquare, targetSquare) {
    setPromoting(null);
    if (locked || !canDrag) return false;
    const moved = attemptMove(sourceSquare, targetSquare);
    setSelectedSquare(null);
    return moved;
  }

  // Where the selected piece can go: a dot on an empty square, a ring on a
  // square with a piece to take (or the en passant square).
  const dots = useMemo(() => {
    if (!selectedSquare || !settings.showLegalMoves) return {};
    const pieces = piecesOf(shownFen);
    const mover = pieces[selectedSquare];
    const out = {};
    for (const sq of legalTargetSquares) {
      const ep = !pieces[sq] && (mover === "P" || mover === "p") && sq[0] !== selectedSquare[0];
      out[sq] = pieces[sq] || ep ? "ring" : "dot";
    }
    return out;
  }, [selectedSquare, legalTargetSquares, shownFen, settings.showLegalMoves]);

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
    for (const uci of premoveActive ? queue || [] : []) {
      for (const sq of [uci.slice(0, 2), uci.slice(2, 4)]) styles[sq] = { ...(styles[sq] || {}), backgroundColor: "rgba(239, 68, 68, 0.5)" };
    }
    if (pending) {
      for (const sq of [pending.from, pending.to]) styles[sq] = { ...(styles[sq] || {}), backgroundColor: "rgba(96, 165, 250, 0.5)" };
    }
    if (selectedSquare) {
      styles[selectedSquare] = { ...(styles[selectedSquare] || {}), backgroundColor: "rgba(250, 204, 21, 0.6)" };
    }
    return styles;
  }, [lastMove, lastMoveColor, hint, selectedSquare, settings.highlightLastMove, queue, premoveActive, pending]);

  const position = pending ? previewPlacement(state.fen, pending.uci) : shownFen;

  function confirm() {
    const uci = pending.uci;
    setPending(null);
    onPlayerMove(uci);
  }

  return (
    <div className="board-wrap">
      <ThemedBoard
        id="DyanisBoard"
        position={position}
        orientation={orientation}
        onPieceDrop={handlePieceDrop}
        onPieceDragBegin={handleDragBegin}
        onSquareClick={handleSquareClick}
        onSquareRightClick={() => {
          if (pending) setPending(null);
          else premove?.onClear?.();
        }}
        isDraggablePiece={isDraggable}
        customSquareStyles={squareStyles}
        dots={dots}
        {...(badge !== undefined ? { badge } : {})}
        {...(arrows ? { customArrows: arrows } : {})}
        promotion={promoting ? { square: promoting.to, letters: promoting.letters, side: pieceSide(state.fen, promoting.from), onPick: choosePromotion } : null}
        onPromotionCheck={() => false} // the library's own dialog stays shut: the board draws its own picker
      />
      {pending && (
        <div className="board-confirm" role="group" aria-label="Подтверждение хода">
          <button type="button" className="btn btn--primary" onClick={confirm} autoFocus>
            <Icon name="check" size={16} /> Подтвердить ход
          </button>
          <button type="button" className="btn" onClick={() => setPending(null)}>
            <Icon name="close" size={16} /> Отмена
          </button>
        </div>
      )}
    </div>
  );
}

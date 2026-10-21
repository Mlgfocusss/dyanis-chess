import { createContext, forwardRef, useContext, useMemo } from "react";
import { Chessboard } from "react-chessboard";
import { useElementWidth } from "../hooks/useElementWidth.js";
import { useSettings, ANIMATION_MS } from "../settings/SettingsContext.jsx";
import { getBoardTheme, getPieceSet } from "../themes/index.js";
import ClassBadge from "../review/ClassBadge.jsx";
import ArrowLayer from "./ArrowLayer.jsx";
import PromotionPicker from "./PromotionPicker.jsx";
import { pickerRunsUp } from "./promotion.js";

const FILES = ["a", "b", "c", "d", "e", "f", "g", "h"];
const GUTTER = 20; // width of the coordinate strip when coordinates sit outside

// react-chessboard props for a given settings object: square colours or
// pictures, custom pieces, animation. Pure, so the settings page can build
// a preview from a draft that is not saved yet.
export function buildBoardProps(settings) {
  const theme = getBoardTheme(settings.boardTheme);
  const set = getPieceSet(settings.pieceSet);

  let light = {};
  let dark = {};
  let boardStyle = {};
  let notation = {};
  if (theme.kind === "color") {
    light = { backgroundColor: theme.light };
    dark = { backgroundColor: theme.dark };
  } else if (theme.kind === "tiles") {
    light = { backgroundImage: `url("${theme.light}")`, backgroundSize: "cover" };
    dark = { backgroundImage: `url("${theme.dark}")`, backgroundSize: "cover" };
    notation = { color: "#fff", textShadow: "0 0 3px #000, 0 0 2px #000", fontWeight: 700 };
  } else {
    // a picture of the whole board behind transparent squares
    light = { backgroundColor: "transparent" };
    dark = { backgroundColor: "transparent" };
    boardStyle = { backgroundImage: `url("${theme.image}")`, backgroundSize: "100% 100%" };
    notation = { color: "#fff", textShadow: "0 0 3px #000, 0 0 2px #000", fontWeight: 700 };
  }

  let customPieces;
  if (!set.builtin) {
    customPieces = {};
    for (const [code, url] of Object.entries(set.files)) {
      customPieces[code] = ({ squareWidth }) => (
        <div
          style={{
            width: squareWidth,
            height: squareWidth,
            backgroundImage: `url("${url}")`,
            backgroundSize: "100% 100%",
            backgroundRepeat: "no-repeat",
          }}
        />
      );
    }
  }

  return {
    customLightSquareStyle: light,
    customDarkSquareStyle: dark,
    customBoardStyle: { borderRadius: "10px", boxShadow: "var(--shadow-board)", ...boardStyle },
    customNotationStyle: notation,
    customPieces,
    animationDuration: ANIMATION_MS[settings.animation] ?? ANIMATION_MS.medium,
    showBoardNotation: settings.coordinates === "inside",
  };
}

// What is drawn over single squares: a move-quality badge (top right corner),
// the dots that mark where the selected piece can go (a dot on an empty
// square, a ring around a piece that can be taken) and the choice of the piece
// for a promotion (on the square the pawn goes to). The square renderer is one
// fixed component and reads what to draw from this context, so
// react-chessboard does not rebuild its squares with every move.
const OverlayCtx = createContext(null);
const OverlaySquare = forwardRef(function OverlaySquare({ children, square, style }, ref) {
  const o = useContext(OverlayCtx);
  const b = o?.badge;
  const lifted = o?.lift?.square === square ? "sq-lifted" : "";
  const extra = o?.classes?.[square] || "";
  const dot = o?.dots?.[square];
  const promo = o?.promotion?.square === square ? o.promotion : null;
  return (
    <div ref={ref} className={[lifted, extra].filter(Boolean).join(" ") || undefined}
      style={{
        ...style,
        position: "relative",
        ...(promo ? { zIndex: 30 } : {}),
        ...(o?.lift?.square === square && o.lift.x !== undefined ? { "--lift-x": o.lift.x + "px", "--lift-y": o.lift.y + "px" } : {}),
      }}
    >
      {o?.marks?.[square] && <span className="sq-mark" />}
      {children}
      {promo && <PromotionPicker {...promo} />}
      {dot && <span className={`sq-${dot}`} />}
      {b && b.square === square && (
        <span className="sq-badge">
          <ClassBadge cls={b.cls} size={b.size} />
        </span>
      )}
    </div>
  );
});

// The chessboard of the whole site: fills its container, wears the user's
// look and draws coordinates outside the board when asked to. Pass
// `settings` to override the saved ones (settings page preview); every
// other prop goes to react-chessboard. dots: { square: "dot" | "ring" }.
// promotion: { square, letters, side, onPick } opens the choice of the piece on that square.
// marks: { square: true } squares the person marked red with the right button; lift: the square
// { square, x?, y? } whose piece is picked up (it rises a little, as on chess.com; x and y shift
// it under the pointer). squareClasses: { square: "class names" } added to the square's box (the chess
// variants mark the king in check, squares hidden by fog); extraPieces: { xD: ({ squareWidth }) => node }
// renders pieces the library does not know (the duck), over the usual set; keep it the same object
// between renders, the library rebuilds the board when it changes.
// engineArrows: [{from, to, kind, width, opacity}] drawn over the board as the engine's arrows of a review
// (review/arrows.js); undefined = this board has none. Pass an array (empty when there are none) on a
// board that has them at times: the board keeps its place in the page whether there are arrows or not.
export default function ThemedBoard({ settings: override, orientation = "white", badge, dots, promotion, marks, lift, squareClasses, extraPieces, engineArrows, ...rest }) {
  const saved = useSettings().settings;
  const settings = override || saved;
  const [ref, width] = useElementWidth();
  const themeProps = useMemo(
    () => buildBoardProps(settings),
    [settings.boardTheme, settings.pieceSet, settings.animation, settings.coordinates] // eslint-disable-line react-hooks/exhaustive-deps
  );

  const outside = settings.coordinates === "outside";
  const boardWidth = Math.max(120, width - (outside ? GUTTER : 0));

  // badge: undefined = this board has none; null = none now; {square, cls} = draw it.
  const overlay = useMemo(
    () => ({
      badge: badge ? { ...badge, size: Math.max(16, Math.round((boardWidth / 8) * 0.3)) } : null,
      dots,
      marks,
      lift,
      classes: squareClasses,
      promotion: promotion
        ? {
            ...promotion,
            size: Math.round(boardWidth / 8),
            up: pickerRunsUp(promotion.square, orientation),
            set: getPieceSet(settings.pieceSet),
          }
        : null,
    }),
    [badge, dots, marks, lift, squareClasses, promotion, boardWidth, orientation, settings.pieceSet]
  );
  const customPieces = useMemo(
    () => (extraPieces ? { ...extraPieces, ...themeProps.customPieces } : themeProps.customPieces),
    [extraPieces, themeProps.customPieces]
  );
  const chessboard = (
    <OverlayCtx.Provider value={overlay}>
      <Chessboard
        boardWidth={boardWidth}
        boardOrientation={orientation}
        {...themeProps}
        customPieces={customPieces}
        {...(badge !== undefined || dots !== undefined || promotion !== undefined || marks !== undefined || lift !== undefined || squareClasses !== undefined
          ? { customSquare: OverlaySquare }
          : {})}
        {...rest}
      />
    </OverlayCtx.Provider>
  );
  const board = engineArrows ? (
    <div className="tb-board">
      {chessboard}
      <ArrowLayer arrows={engineArrows} orientation={orientation} />
    </div>
  ) : (
    chessboard
  );

  if (!outside) {
    return (
      <div ref={ref} className="themed-board">
        {board}
      </div>
    );
  }

  const files = orientation === "white" ? FILES : [...FILES].reverse();
  const ranks = orientation === "white" ? [8, 7, 6, 5, 4, 3, 2, 1] : [1, 2, 3, 4, 5, 6, 7, 8];
  return (
    <div ref={ref} className="themed-board">
      <div className="tb-frame" style={{ width: boardWidth + GUTTER }}>
        <div className="tb-ranks" style={{ height: boardWidth, width: GUTTER }}>
          {ranks.map((r) => (
            <span key={r}>{r}</span>
          ))}
        </div>
        {board}
        <div className="tb-files" style={{ width: boardWidth }}>
          {files.map((f) => (
            <span key={f}>{f}</span>
          ))}
        </div>
      </div>
    </div>
  );
}

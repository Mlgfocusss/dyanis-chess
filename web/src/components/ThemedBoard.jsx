import { useMemo } from "react";
import { Chessboard } from "react-chessboard";
import { useElementWidth } from "../hooks/useElementWidth.js";
import { useSettings, ANIMATION_MS } from "../settings/SettingsContext.jsx";
import { getBoardTheme, getPieceSet } from "../themes/index.js";

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
    customBoardStyle: { borderRadius: "10px", boxShadow: "0 12px 32px rgba(2, 6, 23, 0.5)", ...boardStyle },
    customNotationStyle: notation,
    customPieces,
    animationDuration: ANIMATION_MS[settings.animation] ?? ANIMATION_MS.medium,
    showBoardNotation: settings.coordinates === "inside",
  };
}

// The chessboard of the whole site: fills its container, wears the user's
// look and draws coordinates outside the board when asked to. Pass
// `settings` to override the saved ones (settings page preview); every
// other prop goes to react-chessboard.
export default function ThemedBoard({ settings: override, orientation = "white", ...rest }) {
  const saved = useSettings().settings;
  const settings = override || saved;
  const [ref, width] = useElementWidth();
  const themeProps = useMemo(
    () => buildBoardProps(settings),
    [settings.boardTheme, settings.pieceSet, settings.animation, settings.coordinates] // eslint-disable-line react-hooks/exhaustive-deps
  );

  const outside = settings.coordinates === "outside";
  const boardWidth = Math.max(120, width - (outside ? GUTTER : 0));

  const board = (
    <Chessboard boardWidth={boardWidth} boardOrientation={orientation} {...themeProps} {...rest} />
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

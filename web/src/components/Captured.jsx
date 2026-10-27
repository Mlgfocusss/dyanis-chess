import { useMemo } from "react";
import { useSettings } from "../settings/SettingsContext.jsx";
import { getPieceSet } from "../themes/index.js";
import { ORDER, materialOf } from "./material.js";
import { useT } from "../i18n/index.js";

// Without a picture of the piece, a filled glyph is drawn (the colour comes from
// CSS); the text presentation selector keeps ♟ from turning into an emoji.
const GLYPH = { q: "♛", r: "♜", b: "♝", n: "♞", p: "♟" };

// The pieces a side has taken, and "+3" when it is ahead in material. side is
// whose line this is: "white" shows the black pieces White has captured. When
// the person has chosen a set of pieces with pictures, those are drawn.
// leadOnly shows the lead without the pieces (a puzzle starts mid-game).
export default function Captured({ fen, side, leadOnly = false }) {
  const t = useT();
  const { settings } = useSettings();
  const set = getPieceSet(settings.pieceSet);
  const m = useMemo(() => materialOf(fen), [fen]);
  const taken = side === "white" ? m.white : m.black;
  const lead = side === "white" ? m.diff : -m.diff;
  const theirs = side === "white" ? "black" : "white";
  const pieces = leadOnly ? [] : ORDER.filter((k) => taken[k] > 0);
  if (!pieces.length && lead <= 0) return null;
  return (
    <span className="captured" title={lead > 0 ? t("captured.lead", { lead }) : t("captured.title")}>
      {pieces.map((k) => {
        const picture = !set.builtin && set.files?.[(theirs === "white" ? "w" : "b") + k.toUpperCase()];
        return (
          <span key={k} className={`captured__group captured__group--${theirs}`} aria-label={`${t("piece.plural." + k)}: ${taken[k]}`}>
            {Array.from({ length: taken[k] }, (_, i) =>
              picture ? (
                <img key={i} className="captured__img" src={picture} alt="" draggable={false} />
              ) : (
                <span key={i} className="captured__piece">
                  {GLYPH[k]}
                  {"︎"}
                </span>
              )
            )}
          </span>
        );
      })}
      {lead > 0 && <span className="captured__lead">+{lead}</span>}
    </span>
  );
}

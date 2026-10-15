import Icon from "../ui/Icon.jsx";
import { BADGE_IMAGES } from "./badges/index.js";
import { CLASSES } from "./classes.js";

// Badge of a move class. Colour is never the only signal: the glyph and the
// title say the same thing. If a picture for the class is in review/badges/ it
// is shown as it is; otherwise the badge is drawn here. size (pixels)
// overrides the two stock sizes, for the badge drawn on a board square.
export default function ClassBadge({ cls, large = false, size = 0 }) {
  const c = CLASSES[cls];
  if (!c) return null;

  // A picture is shown bigger than the drawn badge (it has detail to show);
  // 256 px sources stay sharp at these sizes.
  const picture = BADGE_IMAGES[cls];
  const px = size || (picture ? (large ? 48 : 30) : large ? 32 : 22);
  if (picture) {
    return (
      <img
        className="badge-img"
        src={picture}
        width={px}
        height={px}
        alt={c.label}
        title={c.label}
        draggable={false}
      />
    );
  }

  const custom = size
    ? { width: size, height: size, minWidth: size, borderRadius: size / 2, fontSize: Math.max(10, size * 0.5) }
    : null;
  const iconSize = size ? Math.round(size * 0.58) : large ? 16 : 12;
  return (
    <span
      className={`badge${large ? " badge--lg" : ""}`}
      style={{ background: c.color, color: c.ink, ...custom }}
      title={c.label}
    >
      {c.icon ? <Icon name={c.icon} size={iconSize} /> : c.text}
    </span>
  );
}

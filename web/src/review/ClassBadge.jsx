import Icon from "../ui/Icon.jsx";
import { CLASSES } from "./classes.js";

// Round badge of a move class. Colour is never the only signal: the glyph
// and the title say the same thing.
export default function ClassBadge({ cls, large = false }) {
  const c = CLASSES[cls];
  if (!c) return null;
  return (
    <span
      className={`badge${large ? " badge--lg" : ""}`}
      style={{ background: c.color, color: c.ink }}
      title={c.label}
    >
      {c.icon ? <Icon name={c.icon} size={large ? 16 : 12} /> : c.text}
    </span>
  );
}

import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";
import { botText } from "./botText.js";

// A bot has no photo: a round badge in its own colour (taken from its id)
// with its initials; the engine itself gets the chip.
function hue(id) {
  let h = 0;
  for (const c of id) h = (h * 31 + c.charCodeAt(0)) % 360;
  return h;
}

export default function BotAvatar({ bot, size = 40 }) {
  const t = useT();
  const id = bot?.id || bot?.bot || "bot";
  const name = botText(t, id, "name", bot?.name || "");
  const h = hue(id);
  const initials = name
    .split(/\s+/)
    .map((w) => w[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
  return (
    <span
      className="bot-avatar"
      aria-hidden="true"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.4,
        background: `linear-gradient(145deg, hsl(${h} 55% 52%), hsl(${(h + 40) % 360} 60% 34%))`,
      }}
    >
      {id === "max" ? <Icon name="bot" size={size * 0.55} /> : initials}
    </span>
  );
}

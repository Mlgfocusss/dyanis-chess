import { useT } from "../i18n/index.js";
import { KNOWN_CATEGORIES } from "./logic.js";

// The symbol of each category, drawn on the 24x24 grid (stroke only, like ui/Icon.jsx). They are kept here, not in
// Icon.jsx, because the badge is the only place that uses them.
const SYMBOLS = {
  // a board with two filled squares
  games: (
    <>
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M3 12h18M12 3v18" />
      <rect x="3" y="3" width="9" height="9" className="ab__fill" stroke="none" />
      <rect x="12" y="12" width="9" height="9" className="ab__fill" stroke="none" />
    </>
  ),
  bots: (
    <>
      <rect x="4" y="4" width="16" height="16" rx="2" />
      <rect x="9" y="9" width="6" height="6" />
      <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
    </>
  ),
  rating: (
    <>
      <polyline points="3 17 9 11 13 15 21 7" />
      <polyline points="15 7 21 7 21 13" />
    </>
  ),
  puzzles: (
    <>
      <circle cx="12" cy="12" r="10" />
      <circle cx="12" cy="12" r="6" />
      <circle cx="12" cy="12" r="2" />
    </>
  ),
  review: (
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="M21 21l-5-5" />
      <path d="M8 11h2l1.2-2.5 1.6 5 1.2-2.5h1" />
    </>
  ),
  openings: (
    <>
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
    </>
  ),
  lessons: (
    <>
      <path d="M22 10 12 5 2 10l10 5 10-5z" />
      <path d="M6 12v5c3 2.5 9 2.5 12 0v-5" />
    </>
  ),
  community: (
    <>
      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
    </>
  ),
  streaks: <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.07-2.14-.22-4.05 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.15.43-2.29 1-3a2.5 2.5 0 0 0 2.5 2.5z" />,
  // a category the page does not know yet: a star
  other: <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />,
};

const PIPS = { bronze: 1, silver: 2, gold: 3 };

// The medal of an achievement: a shield with the symbol of its category; the number of dots under the
// symbol is the tier (so the grade does not rest on colour alone). An unearned badge is drawn pale.
export default function AchievementBadge({ category, tier = "bronze", unlocked = false, size = 64, title }) {
  const t = useT();
  const pips = PIPS[tier] || 1;
  const symbol = SYMBOLS[KNOWN_CATEGORIES.has(category) ? category : "other"];
  const label = title ? t("achievements.badgeLabel", { name: title, tier: t(`achievements.tier.${tier}`) }) : undefined;
  return (
    <svg
      className={`ab ab--${tier}${unlocked ? " ab--on" : " ab--off"}`}
      width={size}
      height={Math.round((size * 72) / 64)}
      viewBox="0 0 64 72"
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      focusable="false"
    >
      <path className="ab__body" d="M32 3 57 12v22c0 16-11 28-25 35C18 62 7 50 7 34V12z" />
      <path className="ab__ring" d="M32 9 51 16v18c0 12-8 21-19 27-11-6-19-15-19-27V16z" />
      <g className="ab__sym" transform="translate(19 17) scale(1.083)">
        {symbol}
      </g>
      {Array.from({ length: pips }, (_, i) => (
        <circle key={i} className="ab__pip" cx={32 + (i - (pips - 1) / 2) * 7} cy="51" r="2.4" />
      ))}
    </svg>
  );
}

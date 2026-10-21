// Inline SVG icons (Lucide-style paths, 24x24 grid, stroke only).
const PATHS = {
  review: <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />,
  bot: (
    <>
      <rect x="4" y="4" width="16" height="16" rx="2" />
      <rect x="9" y="9" width="6" height="6" />
      <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3" />
    </>
  ),
  online: (
    <>
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
    </>
  ),
  puzzles: (
    <>
      <circle cx="12" cy="12" r="10" />
      <circle cx="12" cy="12" r="6" />
      <circle cx="12" cy="12" r="2" />
    </>
  ),
  openings: (
    <>
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
    </>
  ),
  coach: <path d="M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9zM19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8z" />,
  star: <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />,
  first: <path d="M6 5v14M19 5l-9 7 9 7z" />,
  prev: <path d="M15 5l-8 7 8 7" />,
  next: <path d="M9 5l8 7-8 7" />,
  last: <path d="M18 5v14M5 5l9 7-9 7z" />,
  flip: <path d="M7 4v16M7 20l-3-3M7 20l3-3M17 20V4M17 4l-3 3M17 4l3 3" />,
  upload: <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12" />,
  back: <path d="M19 12H5M12 19l-7-7 7-7" />,
  board: (
    <>
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M3 12h18M12 3v18" />
    </>
  ),
  check: <polyline points="20 6 9 17 4 12" />,
  copy: (
    <>
      <rect x="9" y="9" width="12" height="12" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </>
  ),
  expand: <path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" />,
  shrink: <path d="M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7" />,
  sliders: <path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6" />,
  monitor: (
    <>
      <rect x="2" y="3" width="20" height="14" rx="2" />
      <path d="M8 21h8M12 17v4" />
    </>
  ),
  lock: (
    <>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </>
  ),
  logout: <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" />,
  user: (
    <>
      <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
      <circle cx="12" cy="7" r="4" />
    </>
  ),
  list: <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />,
  trash: <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6M10 11v6M14 11v6" />,
  thumb: <path d="M7 10v12M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2a3.13 3.13 0 0 1 3 3.88Z" />,
  trophy: (
    <>
      <path d="M8 21h8M12 17v4M7 4h10v5a5 5 0 0 1-10 0V4z" />
      <path d="M17 6h3a2 2 0 0 1-2 4h-1M7 6H4a2 2 0 0 0 2 4h1" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  play: <polygon points="6 4 20 12 6 20 6 4" />,
  bots: (
    <>
      <rect x="3" y="8" width="18" height="12" rx="3" />
      <path d="M12 8V4M9 14h.01M15 14h.01M9 17.5h6" />
    </>
  ),
  lessons: (
    <>
      <path d="M22 10 12 5 2 10l10 5 10-5z" />
      <path d="M6 12v5c3 2.5 9 2.5 12 0v-5" />
    </>
  ),
  bolt: <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />,
  duel: <path d="M14.5 17.5 3 6V3h3l11.5 11.5M13 19l6-6M16 16l4 4M19 21l2-2M14.5 6.5 18 3h3v3l-3.5 3.5M5 14l4 4M7 17l-3 3M3 19l2 2" />,
  home: (
    <>
      <path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />
    </>
  ),
  chevron: <path d="m6 9 6 6 6-6" />,
  flag: <path d="M4 22V4M4 4h13l-2 4 2 4H4" />,
  close: <path d="M18 6 6 18M6 6l12 12" />,
  crown: (
    <>
      <path d="m2 4 3 12h14l3-12-6 4-4-6-4 6z" />
      <path d="M5 20h14" />
    </>
  ),
  trend: <path d="M22 7 13.5 15.5 8.5 10.5 2 17M16 7h6v6" />,
  // other chess ("Другие шахматы"): a tilted board with a smaller one inside
  variants: (
    <>
      <path d="M12 2.5 21.5 12 12 21.5 2.5 12z" />
      <path d="M12 7.5 16.5 12 12 16.5 7.5 12z" />
    </>
  ),
  // Crazyhouse: a piece dropped into a tray (the pocket); Bughouse: two boards and the exchange of pieces between them
  crazyhouse: (
    <>
      <path d="M12 3v9M8.5 8.5 12 12l3.5-3.5" />
      <path d="M4 14v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4" />
    </>
  ),
  bughouse: (
    <>
      <rect x="2.5" y="4" width="7.5" height="9" rx="1" />
      <rect x="14" y="11" width="7.5" height="9" rx="1" />
      <path d="M13 6h6M16.5 3.5 19 6l-2.5 2.5M11 18H5M7.5 15.5 5 18l2.5 2.5" />
    </>
  ),
  // fog of war: a cloud over two lines of mist
  fog: (
    <>
      <path d="M4 14.9A7 7 0 1 1 15.7 8h1.8a4.5 4.5 0 0 1 2.5 8.2" />
      <path d="M16 17H7M17 21H9" />
    </>
  ),
  flame: <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.07-2.14-.22-4.05 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.15.43-2.29 1-3a2.5 2.5 0 0 0 2.5 2.5z" />,
  // Atomic chess: the atom (a nucleus and two orbits); King of the Hill: a mountain with a flag; Horde: three pawns
  atomic: (
    <>
      <circle cx="12" cy="12" r="1.2" />
      <path d="M20.2 20.2c2.04-2.03.02-7.36-4.5-11.9-4.54-4.52-9.87-6.54-11.9-4.5-2.04 2.03-.02 7.36 4.5 11.9 4.54 4.52 9.87 6.54 11.9 4.5Z" />
      <path d="M15.7 15.7c4.52-4.54 6.54-9.87 4.5-11.9-2.03-2.04-7.36-.02-11.9 4.5-4.52 4.54-6.54 9.87-4.5 11.9 2.03 2.04 7.36.02 11.9-4.5Z" />
    </>
  ),
  hill: (
    <>
      <path d="m2 21 7-13 4 7 2.5-4L22 21z" />
      <path d="M9 8V2.5l4 1.5-4 1.5" />
    </>
  ),
  horde: (
    <>
      <circle cx="5" cy="7.5" r="2" />
      <path d="M3.4 17.5 4 12.5h2l.6 5M2.5 20.5h5" />
      <circle cx="12" cy="7.5" r="2" />
      <path d="M10.4 17.5 11 12.5h2l.6 5M9.5 20.5h5" />
      <circle cx="19" cy="7.5" r="2" />
      <path d="M17.4 17.5 18 12.5h2l.6 5M16.5 20.5h5" />
    </>
  ),
  // Chess960: two arrows crossing (the pieces are shuffled); Blindfold: an eye crossed out
  chess960: <path d="M16 3h5v5M4 20 21 3M21 16v5h-5M15 15l6 6M4 4l5 5" />,
  blindfold: (
    <>
      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 10 8 10 8a13.16 13.16 0 0 1-1.67 2.68M6.61 6.61A13.53 13.53 0 0 0 2 12s3 8 10 8a9.74 9.74 0 0 0 5.39-1.61" />
      <path d="M14.12 14.12A3 3 0 1 1 9.88 9.88M2 2l20 20" />
    </>
  ),
  // duck chess: a duck seen from the side (head, beak, back, tail)
  duck: (
    <>
      <path d="M4 15c0 3.5 3 6 7 6s7-2.5 7-6c0-1.6-.9-2.7-2.2-3L15 11V7.5a3.5 3.5 0 0 0-7 0V11c-2.5.4-4 2.2-4 4z" />
      <path d="M15 7.5h3.5L15 9.5M11.5 6h.01" />
    </>
  ),
  // dice chess: a die showing five
  dice: (
    <>
      <rect x="3.5" y="3.5" width="17" height="17" rx="3.5" />
      <path d="M8.5 8.5h.01M15.5 8.5h.01M12 12h.01M8.5 15.5h.01M15.5 15.5h.01" />
    </>
  ),
  // review: pause (play is "play"), link, a branch of a line, a warning triangle (the next mistake), share
  pause: <path d="M8 4v16M16 4v16" />,
  link: (
    <>
      <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
      <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
    </>
  ),
  branch: (
    <>
      <path d="M6 3v12M18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z" />
      <path d="M18 9a9 9 0 0 1-9 9" />
    </>
  ),
  alert: <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0zM12 9v4M12 17h.01" />,
  share: <path d="M4 12v7a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-7M16 6l-4-4-4 4M12 2v13" />,
  // chess pieces for the faces of the dice (dice chess): filled silhouettes in the current colour
  pieceP: (
    <>
      <circle cx="12" cy="7.5" r="3.2" fill="currentColor" />
      <path d="M8.5 19.5c0-4 1.2-6 3.5-7 2.3 1 3.5 3 3.5 7z" fill="currentColor" />
      <path d="M6.5 20.5h11" />
    </>
  ),
  pieceN: <path d="M6.5 20.5h11.5v-6.5c0-4.5-3-7.500-7-7.5-3.500 1.500-6 4.500-7.500 8l2 1.500c1.300-1.700 3-2.800 5.500-3-2.200 2.500-2.800 4.500-3 7.500z" fill="currentColor" />,
  pieceB: (
    <>
      <circle cx="12" cy="4.5" r="1.6" fill="currentColor" />
      <path d="M12 6.500c-3 2.200-4.500 5-4.500 7.500 0 1.500.8 2.500 2 3h5c1.200-.5 2-1.500 2-3 0-2.500-1.500-5.300-4.500-7.500z" fill="currentColor" />
      <path d="M7 20.500h10M9 17.500v3M15 17.500v3" />
    </>
  ),
  pieceR: <path d="M6.500 20.500h11v-2L16 17V10l1.500-1.500v-4h-2.300v2h-2v-2h-2.400v2h-2v-2H6.500v4L8 10v7l-1.500 1.500z" fill="currentColor" />,
  pieceQ: (
    <>
      <path d="M5.500 8.500 7.500 17h9l2-8.500-4 3.500-2.500-6-2.500 6z" fill="currentColor" />
      <path d="M6.500 20.500h11V18h-11z" fill="currentColor" />
      <circle cx="5.500" cy="7" r="1.300" fill="currentColor" />
      <circle cx="12" cy="4.500" r="1.300" fill="currentColor" />
      <circle cx="18.500" cy="7" r="1.300" fill="currentColor" />
    </>
  ),
  pieceK: (
    <>
      <path d="M12 3v5M9.500 5.500h5" />
      <path d="M12 8c-3 2.500-5 5-5 8.500h10c0-3.500-2-6-5-8.500z" fill="currentColor" />
      <path d="M6.500 20.500h11v-3h-11z" fill="currentColor" />
    </>
  ),
};

export default function Icon({ name, size = 20, className = "" }) {
  return (
    <svg
      className={`icon ${className}`}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden="true"
      focusable="false"
    >
      {PATHS[name]}
    </svg>
  );
}

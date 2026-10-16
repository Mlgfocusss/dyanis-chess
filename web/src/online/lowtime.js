// The warning of a clock about to run out, with no imports, so `npm test` covers it.

// The seconds left at which a beep sounds: ten, then each of the last three.
export const BEEP_AT = [10, 3, 2, 1];

// Whether a beep is due as the clock goes from `prevSec` to `sec` whole seconds left (ms rounded up).
// Only a falling clock beeps, and each second once.
export function beepDue(prevSec, sec) {
  return sec < prevSec && BEEP_AT.includes(sec);
}

// The clock is shown as running low under this many milliseconds.
export const LOW_MS = 20000;

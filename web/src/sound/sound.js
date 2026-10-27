// Move sounds. The standard theme is synthesised with the Web Audio API, so
// the site has sound without any audio files; a theme folder in
// src/themes/sounds/<name>/ (move.mp3, capture.mp3, ...) replaces it, kind
// by kind: a kind the folder lacks falls back to the synthesised one.
import { getSoundTheme } from "../themes/index.js";

export { soundKindForSan } from "./kinds.js";

let ctx = null;
function audio() {
  if (!ctx) {
    const AC = window.AudioContext || window.webkitAudioContext;
    if (!AC) return null;
    ctx = new AC();
  }
  if (ctx.state === "suspended") ctx.resume().catch(() => {});
  return ctx;
}

const MASTER = 0.35;

// A short burst of filtered noise: the "click" of a piece on wood.
function noise(ac, t, dur, gain, hp) {
  const n = Math.max(1, Math.floor(ac.sampleRate * dur));
  const buf = ac.createBuffer(1, n, ac.sampleRate);
  const d = buf.getChannelData(0);
  for (let i = 0; i < n; i++) d[i] = (Math.random() * 2 - 1) * (1 - i / n);
  const src = ac.createBufferSource();
  src.buffer = buf;
  const f = ac.createBiquadFilter();
  f.type = "highpass";
  f.frequency.value = hp;
  const g = ac.createGain();
  g.gain.setValueAtTime(gain * MASTER, t);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  src.connect(f).connect(g).connect(ac.destination);
  src.start(t);
}

// A sine tone with a fast decay.
function tone(ac, t, freq, dur, gain, endFreq) {
  const o = ac.createOscillator();
  o.type = "sine";
  o.frequency.setValueAtTime(freq, t);
  if (endFreq) o.frequency.exponentialRampToValueAtTime(endFreq, t + dur);
  const g = ac.createGain();
  g.gain.setValueAtTime(gain * MASTER, t);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  o.connect(g).connect(ac.destination);
  o.start(t);
  o.stop(t + dur + 0.02);
}

function thud(ac, t, strong) {
  tone(ac, t, strong ? 220 : 170, 0.11, strong ? 1 : 0.8, 70);
  noise(ac, t, strong ? 0.07 : 0.04, strong ? 0.9 : 0.55, 1800);
}

const SYNTH = {
  move: (ac, t) => thud(ac, t, false),
  capture: (ac, t) => thud(ac, t, true),
  check: (ac, t) => {
    thud(ac, t, false);
    tone(ac, t + 0.06, 880, 0.12, 0.5);
    tone(ac, t + 0.14, 1175, 0.16, 0.5);
  },
  castle: (ac, t) => {
    thud(ac, t, false);
    thud(ac, t + 0.12, false);
  },
  promote: (ac, t) => {
    thud(ac, t, false);
    [523, 659, 784].forEach((f, i) => tone(ac, t + 0.07 + i * 0.07, f, 0.14, 0.45));
  },
  end: (ac, t) => {
    thud(ac, t, true);
    [440, 349, 262].forEach((f, i) => tone(ac, t + 0.1 + i * 0.16, f, 0.3, 0.5));
  },
  start: (ac, t) => tone(ac, t, 660, 0.16, 0.5),
  lowtime: (ac, t) => tone(ac, t, 1046, 0.09, 0.5),
  // "your move" in a correspondence game: a soft rising three-note chime, unlike any move sound
  notify: (ac, t) => [659, 880, 1175].forEach((f, i) => tone(ac, t + i * 0.13, f, 0.28, 0.55)),
};

// Creates (and wakes) the audio context. Called from a click or a key press, it lets a later sound that
// no gesture started (the chime for a correspondence move that arrives on its own) be heard.
export function unlockAudio() {
  try {
    audio();
  } catch {
    // no audio device
  }
}

const cache = new Map();

export function playSound(kind, themeId = "default") {
  try {
    const theme = getSoundTheme(themeId);
    const url = theme.files && (theme.files[kind] || null);
    if (url) {
      let a = cache.get(url);
      if (!a) {
        a = new Audio(url);
        cache.set(url, a);
      }
      a.currentTime = 0;
      a.play().catch(() => {});
      return;
    }
    const ac = audio();
    if (ac && SYNTH[kind]) SYNTH[kind](ac, ac.currentTime + 0.005);
  } catch {
    // no audio device, or autoplay not yet allowed: silence is fine
  }
}

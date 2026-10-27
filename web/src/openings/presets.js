// Openings to start a repository from: the first moves are put in by themselves. `name` is a key of
// the dictionary (openings2.preset.*), `pgn` the moves of the line (the person's colour decides which of
// them are theirs). Pure data, so that `npm test` covers it.
export const PRESETS = [
  { id: "italian", color: "white", name: "openings2.preset.italian", pgn: "1. e4 e5 2. Nf3 Nc6 3. Bc4 *" },
  { id: "ruy", color: "white", name: "openings2.preset.ruy", pgn: "1. e4 e5 2. Nf3 Nc6 3. Bb5 *" },
  { id: "queens-gambit", color: "white", name: "openings2.preset.queensGambit", pgn: "1. d4 d5 2. c4 *" },
  { id: "london", color: "white", name: "openings2.preset.london", pgn: "1. d4 d5 2. Bf4 *" },
  { id: "english", color: "white", name: "openings2.preset.english", pgn: "1. c4 *" },
  { id: "sicilian", color: "black", name: "openings2.preset.sicilian", pgn: "1. e4 c5 *" },
  { id: "caro-kann", color: "black", name: "openings2.preset.caroKann", pgn: "1. e4 c6 *" },
  { id: "french", color: "black", name: "openings2.preset.french", pgn: "1. e4 e6 *" },
  { id: "slav", color: "black", name: "openings2.preset.slav", pgn: "1. d4 d5 2. c4 c6 *" },
  { id: "kings-indian", color: "black", name: "openings2.preset.kingsIndian", pgn: "1. d4 Nf6 2. c4 g6 *" },
];

export const presetsFor = (color) => PRESETS.filter((p) => p.color === color);
export const presetById = (id) => PRESETS.find((p) => p.id === id) || null;

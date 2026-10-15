// Which sound a move makes, from its SAN ("Nf3", "exd5", "O-O", "e8=Q+",
// "Qh4#"). Plain JavaScript with no imports, so it is covered by `npm test`.
export function soundKindForSan(san) {
  if (san.includes("#")) return "end";
  if (san.includes("+")) return "check";
  if (san.startsWith("O-O")) return "castle";
  if (san.includes("=")) return "promote";
  if (san.includes("x")) return "capture";
  return "move";
}

// Classical chess on the board of the variants: hidden from the menu, it exists so that the base (rules,
// server, room) can be played and tested end to end, and as the smallest example of a definition.
export default {
  id: "classic",
  name: "Классические шахматы",
  short: "Обычные шахматы на доске видов: образец и проверка основы.",
  rules: ["Обычные правила шахмат."],
  hidden: true,
  order: 0,
};

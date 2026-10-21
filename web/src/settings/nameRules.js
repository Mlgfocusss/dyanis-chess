// The rules for a display name, the same as the server's (backend/internal/auth/profile.go): so the
// form can say what is wrong before the request. The server checks again. Plain JavaScript, tested.
export const NAME_MIN = 2;
export const NAME_MAX = 24;

// Trims and folds runs of spaces into one.
export function normalizeName(raw) {
  return String(raw ?? "")
    .trim()
    .replace(/[  ]+/g, " ");
}

export function nameLength(name) {
  return [...name].length; // code points, as the server counts runes
}

// { name, error }: error is a Russian sentence, "" when the name is fine.
export function checkName(raw) {
  const name = normalizeName(raw);
  const n = nameLength(name);
  if (!/^[\p{L}\p{Nd} _.-]*$/u.test(name)) {
    return { name, error: "В имени можно использовать буквы, цифры, пробел, дефис, подчёркивание и точку." };
  }
  if (n < NAME_MIN) return { name, error: `Имя слишком короткое: нужно от ${NAME_MIN} до ${NAME_MAX} символов.` };
  if (n > NAME_MAX) return { name, error: `Имя слишком длинное: нужно от ${NAME_MIN} до ${NAME_MAX} символов.` };
  if (!/[\p{L}\p{Nd}]/u.test(name)) return { name, error: "В имени должна быть хотя бы одна буква или цифра." };
  return { name, error: "" };
}

// Whether what was typed confirms deleting the account: the word or the account's name.
export function confirmsDelete(typed, word, name) {
  const t = String(typed ?? "").trim();
  return t !== "" && (t.toLocaleUpperCase("ru") === word || t === name);
}

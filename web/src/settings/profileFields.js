// Rules and bookkeeping of the open-profile form ("Настройки → Профиль"), the same as the server's
// (backend/internal/auth/fields.go), so the form can say what is wrong before the request. The server checks
// again. Plain JavaScript, tested.
import { avatarChoice } from "../auth/avatarPresets.js";
import { checkName } from "./nameRules.js";
import { tr } from "../i18n/index.js";

export const PERSON_MAX = 40; // first and last name
export const LOCATION_MAX = 60;
export const ABOUT_MAX = 500;

// Characters, as the server counts them (code points).
export function textLength(s) {
  return [...String(s ?? "")].length;
}

// Trims and folds runs of spaces into one; the no-break space is a space. Everything that does not print
// (a tab, a line break, an invisible mark) makes the text invalid: { text, ok }.
function foldLine(raw) {
  const text = String(raw ?? "")
    .replace(/ /g, " ")
    .trim()
    .replace(/ {2,}/g, " ");
  return { text, ok: !/[\p{C}\p{Z}]/u.test(text.replace(/ /g, "")) };
}

// { value, error } of a first or last name; label is the field's name in the message.
export function checkPersonName(raw, label) {
  const { text, ok } = foldLine(raw);
  const chars = tr("profileFields.1", { label });
  if (!ok || !/^[\p{L}\p{M} .'’-]*$/u.test(text)) return { value: text, error: chars };
  if (text && !/\p{L}/u.test(text)) return { value: text, error: tr("profileFields.2", { label }) };
  if (textLength(text) > PERSON_MAX) return { value: text, error: tr("profileFields.3", { label, PERSON_MAX }) };
  return { value: text, error: "" };
}

export function checkLocation(raw) {
  const { text, ok } = foldLine(raw);
  if (!ok) return { value: text, error: tr("profileFields.4") };
  if (textLength(text) > LOCATION_MAX) {
    return { value: text, error: tr("profileFields.5", { LOCATION_MAX }) };
  }
  return { value: text, error: "" };
}

// The "about me" text tidied as the server does: line breaks as \n, spaces at the ends of lines gone, at
// most one empty line in a row, nothing at the ends of the text.
export function normalizeAbout(raw) {
  const lines = String(raw ?? "")
    .replace(/\r\n?/g, "\n")
    .replace(/ /g, " ")
    .trim()
    .split("\n");
  const out = [];
  let blank = 0;
  for (const line of lines) {
    const l = line.replace(/ +$/, "");
    blank = l === "" ? blank + 1 : 0;
    if (blank <= 1) out.push(l);
  }
  return out.join("\n");
}

export function checkAbout(raw) {
  const value = normalizeAbout(raw);
  if (/[^\P{C}\n]/u.test(value)) return { value, error: tr("profileFields.6") };
  if (textLength(value) > ABOUT_MAX) return { value, error: tr("profileFields.7", { ABOUT_MAX }) };
  return { value, error: "" };
}

// "123/500" under the text.
export function aboutCounter(raw) {
  return `${textLength(normalizeAbout(raw))}/${ABOUT_MAX}`;
}

// What the form edits. avatar is the picker's choice: "", "google" or "preset:<id>".
export const FIELDS = ["name", "firstName", "lastName", "location", "about", "country", "avatar"];

// The saved values of an account, in the form's shape.
export function savedValues(user) {
  return {
    name: user?.name ?? "",
    firstName: user?.firstName ?? "",
    lastName: user?.lastName ?? "",
    location: user?.location ?? "",
    about: user?.about ?? "",
    country: user?.country ?? "",
    avatar: avatarChoice(user),
  };
}

// Checks the values the form holds. Returns the tidied values, a message per field with a problem and
// whether the whole form may be sent. (The country and the avatar are chosen from lists: never invalid.)
export function checkProfile(values) {
  const name = checkName(values.name);
  const first = checkPersonName(values.firstName, tr("profileFields.8"));
  const last = checkPersonName(values.lastName, tr("profileFields.9"));
  const location = checkLocation(values.location);
  const about = checkAbout(values.about);
  const tidy = {
    name: name.name,
    firstName: first.value,
    lastName: last.value,
    location: location.value,
    about: about.value,
    country: values.country ?? "",
    avatar: values.avatar ?? "",
  };
  const errors = {};
  if (name.error) errors.name = name.error;
  if (first.error) errors.firstName = first.error;
  if (last.error) errors.lastName = last.error;
  if (location.error) errors.location = location.error;
  if (about.error) errors.about = about.error;
  return { values: tidy, errors, valid: Object.keys(errors).length === 0 };
}

// The body of PUT /api/me/profile: only the fields that differ from the saved ones, {} when none does.
export function diffProfile(saved, values) {
  const patch = {};
  for (const f of FIELDS) {
    if ((values[f] ?? "") !== (saved[f] ?? "")) patch[f] = values[f] ?? "";
  }
  return patch;
}

// "Анна Иванова", "Анна" or "" from the two names.
export function fullName(first, last) {
  return [first, last].map((x) => String(x ?? "").trim()).filter(Boolean).join(" ");
}

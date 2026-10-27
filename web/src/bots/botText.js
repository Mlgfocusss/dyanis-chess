// The words of the bots (name, description, what they say) come from the server's Go registry in
// Russian (backend/internal/bots). In English they are taken from the i18n dictionary by the bot's id
// ("bots.<id>.name" ...); a bot without entries keeps the Russian text from the server.
import { tOr } from "../i18n/index.js";

// field: "name" | "desc" | "greeting" | "onWin" | "onLose"
export function botText(t, id, field, fallback) {
  return id ? tOr(t, `bots.${id}.${field}`, fallback) : fallback;
}

// The name of a bot category ("beginners" ...) from the server's list.
export function botCategoryName(t, cat) {
  return cat ? tOr(t, `bots.cat.${cat.id}`, cat.name) : "";
}

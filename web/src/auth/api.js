// Client of the account API (backend/internal/auth). The session lives in an
// HttpOnly cookie the browser sends by itself, so nothing here handles tokens.

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    const e = new Error(body?.error || `Сервер ответил ${res.status}`);
    e.status = res.status;
    throw e;
  }
  return body;
}

// { user: {...} | null, google: bool }. Throws when the server runs without
// a database (no accounts at all).
export async function getMe() {
  return readJSON(await fetch("/api/me"));
}

export async function startGuest() {
  return (await readJSON(await fetch("/api/auth/guest", { method: "POST" }))).user;
}

export async function logout() {
  await readJSON(await fetch("/api/auth/logout", { method: "POST" }));
}

export async function saveSettings(settings) {
  const res = await fetch("/api/me/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
    keepalive: true, // lets a request started while the tab closes still go out
  });
  if (!res.ok) throw new Error(`Сервер ответил ${res.status}`);
}

const JSON_HEADERS = { "Content-Type": "application/json" };

// Saves the profile fields in the patch (name, firstName, lastName, location, about, country, avatar; each
// optional, an empty string clears one) and resolves to the updated user. Errors carry the server's Russian
// text (400 invalid field, 429 saved too recently).
export async function saveProfile(patch) {
  const res = await fetch("/api/me/profile", { method: "PUT", headers: JSON_HEADERS, body: JSON.stringify(patch) });
  return (await readJSON(res)).user;
}

// Changes the display name; resolves to the updated user.
export function renameMe(name) {
  return saveProfile({ name });
}

// The word the server wants in the body of the deletion request.
export const DELETE_WORD = "УДАЛИТЬ";

// Deletes the signed-in account with its data (409 while a game with a person is going on).
export async function deleteMe() {
  const res = await fetch("/api/me", { method: "DELETE", headers: JSON_HEADERS, body: JSON.stringify({ confirm: DELETE_WORD }) });
  await readJSON(res);
}

export const GOOGLE_LOGIN_URL = "/api/auth/google/login";

// Texts for the ?auth_error=<code> the server redirects back with.
export const AUTH_ERRORS = {
  denied: "Вход отменён.",
  state: "Не удалось подтвердить вход (устарела ссылка). Попробуйте ещё раз.",
  google: "Google не принял вход. Попробуйте ещё раз.",
  server: "Сервер не смог завершить вход. Попробуйте позже.",
  not_configured: "Вход через Google не настроен на этом сервере.",
};

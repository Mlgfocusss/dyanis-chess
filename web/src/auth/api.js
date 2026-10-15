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

export const GOOGLE_LOGIN_URL = "/api/auth/google/login";

// Texts for the ?auth_error=<code> the server redirects back with.
export const AUTH_ERRORS = {
  denied: "Вход отменён.",
  state: "Не удалось подтвердить вход (устарела ссылка). Попробуйте ещё раз.",
  google: "Google не принял вход. Попробуйте ещё раз.",
  server: "Сервер не смог завершить вход. Попробуйте позже.",
  not_configured: "Вход через Google не настроен на этом сервере.",
};

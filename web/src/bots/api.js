// Client of the bots API (backend/internal/play: GET /api/bots, POST /api/games with "bot").
import { useCallback, useEffect, useState } from "react";

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

// { categories: [{id, name}], bots: [...], progress: {botId: {wins, draws, losses, beaten, points}}, points }
export async function listBots() {
  return readJSON(await fetch("/api/bots"));
}

export async function createBotGame({ bot, color, timeMs = 0, incMs = 0 }) {
  const res = await fetch("/api/games", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ bot, color, timeMs, incMs }),
  });
  return (await readJSON(res)).id;
}

// The bots and the person's progress, loaded once and reloadable.
export function useBots(userId) {
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const reload = useCallback(async () => {
    try {
      const d = await listBots();
      setData(d);
      setError("");
      return d;
    } catch (e) {
      setError(e.message);
      return null;
    }
  }, []);
  useEffect(() => {
    reload();
  }, [reload, userId]);
  return { data, error, reload };
}

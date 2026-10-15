import { useCallback, useEffect, useRef, useState } from "react";
import { getGame, socketUrl } from "./api.js";

// One online game: its latest state from the server and the means to act on
// it. The state comes over a WebSocket; if the connection drops (a phone
// changing network, a restarted server) the hook reconnects by itself and the
// server's first message after that is the whole current state again, so
// nothing can be missed or doubled.
//
//   snap     { game, you, receivedAt } or null before the first state
//   status   "loading" | "open" | "reconnecting" | "notfound"
//   notice   the server's last refusal ("такой ход невозможен"), cleared after a few seconds
//   rev      grows after every refusal; a board keyed on it redraws from the true position
//   send     send(type, extra) -> true if the message went out
//
// userId makes the socket start over when the person signs in (the server
// learns who is on the other end when the connection opens).
export function useGame(id, userId) {
  const [snap, setSnap] = useState(null);
  const [status, setStatus] = useState("loading");
  const [notice, setNotice] = useState("");
  const [rev, setRev] = useState(0);
  const ws = useRef(null);
  const noticeTimer = useRef(null);

  useEffect(() => {
    let closed = false;
    let attempt = 0;
    let retry = null;

    const take = (m) => setSnap({ game: m.game, you: m.you || "", receivedAt: performance.now() });

    async function open() {
      if (closed) return;
      try {
        take(await getGame(id)); // the page is useful before the socket is up
      } catch (e) {
        if (e.status === 404) return setStatus("notfound");
        setStatus("reconnecting");
        retry = setTimeout(open, Math.min(5000, 500 * 2 ** attempt++));
        return;
      }
      if (closed) return;
      const sock = new WebSocket(socketUrl(id));
      ws.current = sock;
      sock.onopen = () => {
        attempt = 0;
        setStatus("open");
      };
      sock.onmessage = (ev) => {
        let m;
        try {
          m = JSON.parse(ev.data);
        } catch {
          return;
        }
        if (m.type === "state" && m.game) take(m);
        if (m.type === "error") {
          setNotice(m.error || "Действие отклонено");
          setRev((r) => r + 1);
          clearTimeout(noticeTimer.current);
          noticeTimer.current = setTimeout(() => setNotice(""), 4000);
        }
      };
      sock.onclose = () => {
        if (closed || ws.current !== sock) return;
        ws.current = null;
        setStatus("reconnecting");
        retry = setTimeout(open, Math.min(5000, 500 * 2 ** attempt++));
      };
    }

    // Coming back to a tab that was asleep: do not wait for the next retry.
    const onVisible = () => {
      if (document.visibilityState === "visible" && !ws.current && !closed) {
        clearTimeout(retry);
        open();
      }
    };
    document.addEventListener("visibilitychange", onVisible);

    setStatus("loading");
    open();
    return () => {
      closed = true;
      clearTimeout(retry);
      clearTimeout(noticeTimer.current);
      document.removeEventListener("visibilitychange", onVisible);
      const sock = ws.current;
      ws.current = null;
      sock?.close();
    };
  }, [id, userId]);

  const send = useCallback((type, extra = {}) => {
    const sock = ws.current;
    if (!sock || sock.readyState !== WebSocket.OPEN) return false;
    sock.send(JSON.stringify({ type, ...extra }));
    return true;
  }, []);

  return { snap, status, notice, rev, send };
}

// Time left on both clocks, ticking: [whiteMs, blackMs], or null without a
// clock. The server says how much each side had at the moment it sent the
// state and which side's time is running; we count down from when it arrived.
export function useClocks(snap) {
  const [, tick] = useState(0);
  const running = snap?.game?.clock?.running || "";
  useEffect(() => {
    if (!running) return undefined;
    const t = setInterval(() => tick((n) => n + 1), 100);
    return () => clearInterval(t);
  }, [running, snap]);
  const c = snap?.game?.clock;
  if (!c) return null;
  const spent = running ? performance.now() - snap.receivedAt : 0;
  return [
    Math.max(0, c.whiteMs - (running === "white" ? spent : 0)),
    Math.max(0, c.blackMs - (running === "black" ? spent : 0)),
  ];
}

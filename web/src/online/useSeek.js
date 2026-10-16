import { useCallback, useEffect, useRef, useState } from "react";
import { cancelSeek, checkSeek, seekMatch } from "./api.js";

const POLL_MS = 1000;

// A search for a person to play: start(request) joins the queue, the hook asks the server how it
// goes every second (which also tells the server the person is still there), and onFound gets
// { kind, id } when somebody was found. status is null while not searching, else the server's
// { status: "waiting", waited, window, count }. Leaving the page leaves the queue.
export default function useSeek(onFound) {
  const [status, setStatus] = useState(null);
  const [error, setError] = useState("");
  const active = useRef(false);
  const timer = useRef(null);
  const found = useRef(onFound);
  found.current = onFound;

  const done = useCallback((st) => {
    active.current = false;
    clearTimeout(timer.current);
    setStatus(null);
    if (st) found.current(st);
  }, []);

  const poll = useCallback(async () => {
    if (!active.current) return;
    try {
      const st = await checkSeek();
      if (!active.current) return;
      if (st.status === "found") return done(st);
      if (st.status === "none") {
        done(null);
        setError("Поиск прервался, попробуйте ещё раз.");
        return;
      }
      setStatus(st);
    } catch {
      // a hiccup of the network: ask again
    }
    timer.current = setTimeout(poll, POLL_MS);
  }, [done]);

  const start = useCallback(
    async (req) => {
      setError("");
      active.current = true;
      setStatus({ status: "waiting", waited: 0 });
      try {
        const st = await seekMatch(req);
        if (!active.current) return;
        if (st.status === "found") return done(st);
        setStatus(st);
        timer.current = setTimeout(poll, POLL_MS);
      } catch (e) {
        done(null);
        setError(e.message);
      }
    },
    [done, poll]
  );

  const cancel = useCallback(async () => {
    if (!active.current) return;
    active.current = false;
    clearTimeout(timer.current);
    setStatus(null);
    try {
      const st = await cancelSeek();
      if (st.status === "found") found.current(st); // it was too late to cancel: the game exists
    } catch {
      // the server drops an abandoned search by itself
    }
  }, []);

  useEffect(
    () => () => {
      clearTimeout(timer.current);
      if (active.current) {
        active.current = false;
        cancelSeek().catch(() => {});
      }
    },
    []
  );

  return { status, error, start, cancel };
}

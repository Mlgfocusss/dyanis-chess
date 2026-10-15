import { useCallback, useEffect, useRef, useState } from "react";
import { createAnalysis, getAnalysis } from "./api.js";

const POLL_MS = 400;

// One analysis job: start it from a PGN (or attach to an existing id) and
// watch it until it is done.
//   job    the last state from the server ({status, done, total, report, ...}) or null
//   busy   true from the start until it is done or has failed
//   error  the failure message, "" if none
export function useAnalysis() {
  const [job, setJob] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const timer = useRef(null);
  const alive = useRef(true);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      clearTimeout(timer.current);
    };
  }, []);

  const poll = useCallback(async (id) => {
    try {
      const j = await getAnalysis(id);
      if (!alive.current) return;
      setJob(j);
      if (j.status === "done") return setBusy(false);
      if (j.status === "error") {
        setError(j.error || "Разбор не удался");
        return setBusy(false);
      }
      timer.current = setTimeout(() => poll(id), POLL_MS);
    } catch (e) {
      if (!alive.current) return;
      setError(e.message);
      setBusy(false);
    }
  }, []);

  const reset = useCallback(() => {
    clearTimeout(timer.current);
    setJob(null);
    setError("");
    setBusy(false);
  }, []);

  const start = useCallback(
    async ({ pgn, game = 1, depth = 0 }) => {
      clearTimeout(timer.current);
      setError("");
      setJob(null);
      setBusy(true);
      try {
        const id = await createAnalysis({ pgn, game, depth });
        if (alive.current) poll(id);
      } catch (e) {
        if (!alive.current) return;
        setError(e.message);
        setBusy(false);
      }
    },
    [poll]
  );

  const attach = useCallback(
    (id) => {
      setError("");
      setBusy(true);
      poll(id);
    },
    [poll]
  );

  return { job, error, busy, start, attach, reset };
}

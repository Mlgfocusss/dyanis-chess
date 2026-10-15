import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { AUTH_ERRORS, getMe, logout as apiLogout, startGuest } from "./api.js";

// Who is signed in. status: "loading" until /api/me answers, "ready", or
// "unavailable" when the server has no accounts (it runs without MySQL).
const Ctx = createContext(null);

// The server sends the person back to /?auth_error=<code> when Google
// sign-in fails; read it once and clean the address.
function takeAuthError() {
  const params = new URLSearchParams(window.location.search);
  const code = params.get("auth_error");
  if (code === null) return "";
  params.delete("auth_error");
  const q = params.toString();
  window.history.replaceState(null, "", window.location.pathname + (q ? `?${q}` : "") + window.location.hash);
  return AUTH_ERRORS[code] || "Не удалось войти.";
}

export function AuthProvider({ children }) {
  const [state, setState] = useState({ status: "loading", user: null, google: false });
  const [authError] = useState(takeAuthError);

  useEffect(() => {
    let alive = true;
    getMe().then(
      (me) => alive && setState({ status: "ready", user: me.user, google: me.google }),
      () => alive && setState({ status: "unavailable", user: null, google: false })
    );
    return () => {
      alive = false;
    };
  }, []);

  const guest = useCallback(async () => {
    const user = await startGuest();
    setState((s) => ({ ...s, user }));
    return user;
  }, []);

  const logout = useCallback(async () => {
    await apiLogout();
    setState((s) => ({ ...s, user: null }));
  }, []);

  const value = useMemo(() => ({ ...state, authError, guest, logout }), [state, authError, guest, logout]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useAuth outside AuthProvider");
  return v;
}

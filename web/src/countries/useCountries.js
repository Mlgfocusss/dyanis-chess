import { useEffect, useState } from "react";
import { tr } from "../i18n/index.js";

// The list of countries, fetched once per page load from GET /api/countries (the server is its only
// source; the browser may also keep it for a day) and kept in memory.
let loading = null;

export function loadCountries() {
  if (!loading) {
    loading = fetch("/api/countries")
      .then((res) => {
        if (!res.ok) throw new Error(tr("common.serverReplied", { status: res.status }));
        return res.json();
      })
      .then((body) => (Array.isArray(body?.countries) ? body.countries : []))
      .catch((e) => {
        loading = null; // try again the next time the list is asked for
        throw e;
      });
  }
  return loading;
}

// { countries, ready, error }: countries is [] until the list arrives.
export default function useCountries() {
  const [state, setState] = useState({ countries: [], ready: false, error: "" });
  useEffect(() => {
    let alive = true;
    loadCountries().then(
      (countries) => alive && setState({ countries, ready: true, error: "" }),
      (e) => alive && setState({ countries: [], ready: false, error: e.message }),
    );
    return () => {
      alive = false;
    };
  }, []);
  return state;
}

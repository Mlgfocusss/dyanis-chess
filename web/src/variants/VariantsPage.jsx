import { useEffect, useState } from "react";
import Catalog from "./Catalog.jsx";
import VariantLobby from "./VariantLobby.jsx";
import VariantRoom from "./VariantRoom.jsx";
import { getVariant } from "./registry.js";
import "./variants.css";

// "#variants" is the catalogue, "#variants/<id>" the lobby of a variant (a third part, "quick" or "link",
// says which way to play opens first), "#variants/game/<gameId>" a game (the address to send a friend).
export function parseHash(hash) {
  const [, a = "", b = ""] = hash.replace(/^#/, "").split("/");
  if (a === "game") return { view: "game", id: b };
  if (a) return { view: "lobby", id: a, mode: b };
  return { view: "catalog" };
}

export default function VariantsPage() {
  const [route, setRoute] = useState(() => parseHash(window.location.hash));
  useEffect(() => {
    const onHash = () => setRoute(parseHash(window.location.hash));
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  if (route.view === "game" && route.id) return <VariantRoom key={route.id} id={route.id} />;
  if (route.view === "lobby") {
    const def = getVariant(route.id);
    if (def) return <VariantLobby key={def.id} def={def} mode={route.mode} />;
    return <Catalog missing={route.id} />;
  }
  return <Catalog />;
}

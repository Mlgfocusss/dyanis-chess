import { useEffect, useState } from "react";
import Lobby from "./Lobby.jsx";
import GameRoom from "./GameRoom.jsx";

// "#online" is the lobby, "#online/<id>" a game (the address to send a friend).
function idFromHash() {
  const [page, id] = window.location.hash.slice(1).split("/");
  return page === "online" ? id || "" : "";
}

export default function OnlinePage() {
  const [id, setId] = useState(idFromHash);
  useEffect(() => {
    const onHash = () => setId(idFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return id ? <GameRoom key={id} id={id} /> : <Lobby />;
}

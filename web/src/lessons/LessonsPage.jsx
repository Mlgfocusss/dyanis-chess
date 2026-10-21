import { useEffect, useState } from "react";
import Catalog from "./Catalog.jsx";
import Player from "./Player.jsx";
import { parseHash } from "./lesson.js";
import "./lessons.css";

// The "Уроки" section: the catalogue at #lessons and a lesson at #lessons/<id>. The address is the
// one source of which of the two is shown, so the menu, the links and a typed address all work.
export default function LessonsPage() {
  const [route, setRoute] = useState(() => parseHash(window.location.hash));

  useEffect(() => {
    const onHash = () => {
      if (window.location.hash.startsWith("#lessons")) setRoute(parseHash(window.location.hash));
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  return route.view === "lesson" ? <Player key={route.id} id={route.id} /> : <Catalog />;
}

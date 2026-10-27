import { useEffect, useState } from "react";
import Builder from "./Builder.jsx";
import GamesReview from "./GamesReview.jsx";
import RepList from "./RepList.jsx";
import Trainer from "./Trainer.jsx";
import "./openings.css";
import "./holes.css";

// "#openings" is the list, "#openings/<id>" builds one repository, "#openings/<id>/fill" builds it
// and leads from one hole (a popular reply of the opponent it has no answer to) to the next,
// "#openings/<id>/train" learns it, "#openings/<id>/games" puts the person's games against it, and
// "#openings/<id>/move/<node>[/<uci>]" opens the builder at a move of the tree (and
// first adds the move <uci> after it: what the opponent played in a game).
function routeFromHash() {
  const [page, id, mode, node, uci] = window.location.hash.slice(1).split("/");
  if (page !== "openings") return { id: 0, mode: "" };
  const n = Number(id);
  const route = { id: Number.isInteger(n) && n > 0 ? n : 0, mode: mode === "train" || mode === "games" || mode === "fill" ? mode : "" };
  if (route.id && mode === "move") {
    const at = Number(node);
    route.start = { node: Number.isInteger(at) && at >= 0 ? at : 0, uci: /^[a-h][1-8][a-h][1-8][qrbn]?$/.test(uci || "") ? uci : "" };
  }
  return route;
}

export default function OpeningsPage() {
  const [route, setRoute] = useState(routeFromHash);
  useEffect(() => {
    const onHash = () => setRoute(routeFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  if (route.id && route.mode === "train") return <Trainer key={route.id} id={route.id} />;
  if (route.id && route.mode === "games") return <GamesReview key={route.id} id={route.id} />;
  if (route.id) return <Builder key={`${route.id}:${route.start?.node}:${route.start?.uci}:${route.mode}`} id={route.id} start={route.start} fillStart={route.mode === "fill"} />;
  return <RepList />;
}

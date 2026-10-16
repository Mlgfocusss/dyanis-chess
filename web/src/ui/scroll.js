// Scrolls an item into view inside its own scrolling box and nowhere else. The
// browser's scrollIntoView also scrolls the whole page, which made the pages with
// a list beside the board jump down to the list at every move.

// The scroll position a box needs so that an item is fully visible, as little
// moved as possible: viewTop and itemTop are measured on the screen, heights in
// pixels, scrollTop is the box's current position.
export function nextScrollTop({ viewTop, viewHeight, itemTop, itemHeight, scrollTop }) {
  const above = itemTop - viewTop; // < 0: the item starts above the visible part
  const below = itemTop + itemHeight - (viewTop + viewHeight); // > 0: it ends below it
  if (above < 0) return scrollTop + above;
  if (below > 0) return scrollTop + Math.min(below, above); // never push the item's top out of sight
  return scrollTop;
}

// The nearest ancestor that scrolls by itself, or null.
function scroller(el) {
  for (let p = el.parentElement; p; p = p.parentElement) {
    const o = getComputedStyle(p).overflowY;
    if ((o === "auto" || o === "scroll") && p.scrollHeight > p.clientHeight) return p;
  }
  return null;
}

export function scrollWithin(el) {
  const box = el && scroller(el);
  if (!box) return;
  const b = box.getBoundingClientRect();
  const r = el.getBoundingClientRect();
  box.scrollTop = nextScrollTop({ viewTop: b.top, viewHeight: box.clientHeight, itemTop: r.top, itemHeight: r.height, scrollTop: box.scrollTop });
}

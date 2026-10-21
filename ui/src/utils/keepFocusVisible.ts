// On a phone the keyboard takes the bottom half of the screen. The board
// scrolls inside its own containers rather than the page, so browsers
// don't bring a field typed into there (a new task at the bottom of a
// card, say) back into view: this does, when the field is focused and
// whenever the visible area changes size as the keyboard opens.
//
// With interactive-widget=resizes-content (index.html) Android shrinks
// the page itself, so scrolling into view is enough. iOS ignores that and
// lays the keyboard over the page, so the field is also lifted above the
// visual viewport's bottom edge.

const MARGIN = 12;

const isField = (el: Element | null): el is HTMLElement =>
  el instanceof HTMLTextAreaElement ||
  (el instanceof HTMLInputElement &&
    !["button", "checkbox", "radio", "submit"].includes(el.type));

function scrollParent(el: HTMLElement): HTMLElement | null {
  for (let p = el.parentElement; p; p = p.parentElement) {
    const { overflowY } = getComputedStyle(p);
    if (
      (overflowY === "auto" || overflowY === "scroll") &&
      p.scrollHeight > p.clientHeight
    ) {
      return p;
    }
  }
  return null;
}

function reveal() {
  const field = document.activeElement;
  if (!isField(field)) return;
  const viewport = window.visualViewport;
  const visible = viewport ? viewport.height : window.innerHeight;

  // The field's form too (its Create button), when there's room for it.
  const form = field.closest("form");
  const target =
    form && form.getBoundingClientRect().height + 2 * MARGIN <= visible
      ? form
      : field;
  target.scrollIntoView({ block: "nearest" });

  if (!viewport) return;
  const bottom = viewport.offsetTop + viewport.height - MARGIN;
  const hidden = target.getBoundingClientRect().bottom - bottom;
  if (hidden > 0) scrollParent(target)?.scrollBy({ top: hidden });
}

export function keepFocusVisible() {
  // After the browser's own scrolling and layout for the change.
  const later = () => requestAnimationFrame(reveal);
  window.visualViewport?.addEventListener("resize", later);

  // Things around the field can grow while it's focused too: each task
  // added from a card's form lands above it and pushes it down.
  const grown = new ResizeObserver(later);
  document.addEventListener("focusin", (e) => {
    grown.disconnect();
    if (!(e.target instanceof Element) || !isField(e.target)) return;
    const stop = scrollParent(e.target);
    for (let p = e.target.parentElement; p && p !== stop; p = p.parentElement) {
      grown.observe(p);
    }
    later();
  });
  document.addEventListener("focusout", () => grown.disconnect());
}

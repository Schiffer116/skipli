import { useEffect } from "react";

const DURATION = 350;

function bezier(x1: number, y1: number, x2: number, y2: number) {
  const at = (a: number, b: number, t: number) =>
    3 * a * (1 - t) ** 2 * t + 3 * b * (1 - t) * t ** 2 + t ** 3;

  return (x: number) => {
    let lo = 0;
    let hi = 1;
    for (let i = 0; i < 20; i++) {
      const mid = (lo + hi) / 2;
      if (at(x1, x2, mid) < x) lo = mid;
      else hi = mid;
    }
    return at(y1, y2, (lo + hi) / 2);
  };
}

const overshot = bezier(0.05, 0.9, 0.1, 1.3);

export function useSwipeSnap(element: HTMLElement | null, enabled: boolean) {
  useEffect(() => {
    if (!element || !enabled) return;

    let startX = 0;
    let startY = 0;
    let startScroll = 0;
    let startTime = 0;
    let axis: "x" | "y" | null = null;
    let frame = 0;

    const pages = () => {
      const left =
        element.getBoundingClientRect().left +
        parseFloat(getComputedStyle(element).paddingLeft);
      return [...element.children]
        .map((child) => child.getBoundingClientRect())
        .filter((rect) => rect.width > 0)
        .map((rect) => element.scrollLeft + rect.left - left);
    };

    const animate = (to: number) => {
      const from = element.scrollLeft;
      const start = performance.now();
      const step = (now: number) => {
        const t = Math.min(1, (now - start) / DURATION);
        element.scrollLeft = from + (to - from) * overshot(t);
        if (t < 1) frame = requestAnimationFrame(step);
      };
      frame = requestAnimationFrame(step);
    };

    const onStart = (e: TouchEvent) => {
      cancelAnimationFrame(frame);
      startX = e.touches[0].clientX;
      startY = e.touches[0].clientY;
      startScroll = element.scrollLeft;
      startTime = performance.now();
      axis = null;
    };

    const onMove = (e: TouchEvent) => {
      const dx = e.touches[0].clientX - startX;
      const dy = e.touches[0].clientY - startY;
      if (!axis && Math.hypot(dx, dy) > 8) {
        axis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      }
      if (axis === "x") element.scrollLeft = startScroll - dx;
    };

    const onEnd = (e: TouchEvent) => {
      if (axis !== "x") return;
      const dx = e.changedTouches[0].clientX - startX;
      const velocity = dx / (performance.now() - startTime);
      const positions = pages();

      let index = 0;
      positions.forEach((position, i) => {
        if (
          Math.abs(position - startScroll) <
          Math.abs(positions[index] - startScroll)
        ) {
          index = i;
        }
      });

      if (
        Math.abs(dx) > element.clientWidth * 0.2 ||
        Math.abs(velocity) > 0.3
      ) {
        index += dx < 0 ? 1 : -1;
      }
      index = Math.max(0, Math.min(positions.length - 1, index));
      animate(positions[index]);
    };

    element.addEventListener("touchstart", onStart, { passive: true });
    element.addEventListener("touchmove", onMove, { passive: true });
    element.addEventListener("touchend", onEnd);
    return () => {
      cancelAnimationFrame(frame);
      element.removeEventListener("touchstart", onStart);
      element.removeEventListener("touchmove", onMove);
      element.removeEventListener("touchend", onEnd);
    };
  }, [element, enabled]);
}

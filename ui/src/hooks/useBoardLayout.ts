import { useEffect, useState } from "react";

export type BoardLayout = "scroll" | "masonry";

const LAYOUT_KEY = "skipli:boardLayout";
const CARD_WIDTH = 320;
const GAP = 16;

function readLayout(): BoardLayout {
  try {
    return localStorage.getItem(LAYOUT_KEY) === "masonry"
      ? "masonry"
      : "scroll";
  } catch {
    return "scroll";
  }
}

export function useBoardLayout() {
  const [layout, setLayout] = useState(readLayout);

  useEffect(() => {
    try {
      localStorage.setItem(LAYOUT_KEY, layout);
    } catch {
      return;
    }
  }, [layout]);

  return [layout, setLayout] as const;
}

export function useColumnCount(element: HTMLElement | null) {
  const [count, setCount] = useState(1);

  useEffect(() => {
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      const width = entry.contentRect.width;
      setCount(Math.max(1, Math.floor((width + GAP) / (CARD_WIDTH + GAP))));
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [element]);

  return count;
}

import { getClientRect } from "@dnd-kit/core";
import { useEffect, useMemo, useState } from "react";

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

// Masonry places item i in column i % columns, stacked under the items
// before it in that column. Everything stays in one container and is moved
// by transform, so a reorder glides instead of remounting cards into
// other columns.
export function useMasonry(element: HTMLElement | null, keys: string[]) {
  const [width, setWidth] = useState(0);
  const [heights, setHeights] = useState<Record<string, number>>({});

  useEffect(() => {
    if (!element) return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(entry.contentRect.width),
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [element]);

  const itemObserver = useMemo(
    () =>
      new ResizeObserver((entries) =>
        setHeights((heights) => {
          const next = { ...heights };
          for (const entry of entries) {
            const key = (entry.target as HTMLElement).dataset.key!;
            next[key] = entry.borderBoxSize[0].blockSize;
          }
          return next;
        }),
      ),
    [],
  );
  useEffect(() => () => itemObserver.disconnect(), [itemObserver]);

  const measure = (node: HTMLElement | null) => {
    if (!node) return;
    itemObserver.observe(node);
    return () => itemObserver.unobserve(node);
  };

  const columns = Math.max(1, Math.floor((width + GAP) / (CARD_WIDTH + GAP)));
  const columnWidth = (width - GAP * (columns - 1)) / columns;
  const bottoms = Array<number>(columns).fill(0);
  const positions = keys.map((key, i) => {
    const column = i % columns;
    const y = bottoms[column];
    bottoms[column] += (heights[key] ?? 0) + GAP;
    return { x: column * (columnWidth + GAP), y, width: columnWidth };
  });

  return {
    columns,
    positions,
    height: Math.max(0, ...bottoms) - GAP,
    measured: keys.every((key) => key in heights),
    measure,
  };
}

// dnd-kit measures drop targets while masonry items are still gliding, which
// makes the pointer seem over the wrong card and swaps cards back and forth.
// Measure where an element is headed instead: shift its current rect by the
// rest of its item's transition.
export function measureMasonryTarget(element: HTMLElement) {
  const rect = getClientRect(element, { ignoreTransform: true });
  const item = element.closest<HTMLElement>("[data-key]");
  if (!item) return rect;

  const current = new DOMMatrix(getComputedStyle(item).transform);
  const target = new DOMMatrix(item.style.transform);
  const dx = target.m41 - current.m41;
  const dy = target.m42 - current.m42;
  return {
    width: rect.width,
    height: rect.height,
    top: rect.top + dy,
    bottom: rect.bottom + dy,
    left: rect.left + dx,
    right: rect.right + dx,
  };
}

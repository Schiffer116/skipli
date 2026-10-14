import { getClientRect } from "@dnd-kit/core";
import { useEffect, useMemo, useState } from "react";

import type { CardType } from "@/pages/Boards/Board/Card";
import { estimateHeight, placeColumns } from "@/utils/masonry";

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

export const CREATE_CARD_KEY = "create-card";

// Masonry draws the card order into columns, each card under the shortest
// column so far (see placeColumns), so any reorder redraws the whole board.
// Every card stays in one container and is moved by transform, so a reorder
// glides instead of remounting cards into other columns.
export function useMasonry(element: HTMLElement | null, cards: CardType[]) {
  const [width, setWidth] = useState(0);
  const [heights, setHeights] = useState<Record<string, number>>({});
  const [ready, setReady] = useState(false);

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

  // No columns until the container is on screen and measured.
  const count =
    element && width > 0
      ? Math.max(1, Math.floor((width + GAP) / (CARD_WIDTH + GAP)))
      : 0;

  // Which column a card goes in uses the task counts as of the last change
  // to the card order or the column count. Adding, deleting or dragging a
  // task only resizes its card in place, rather than sending later cards
  // to other columns, possibly under the pointer mid-drag.
  const ids = cards.map((card) => card.id);
  const order = ids.join();
  const [estimate, setEstimate] = useState({
    count,
    order,
    heights: estimateHeights(cards),
  });
  let estimated = estimate.heights;
  if (estimate.count !== count || estimate.order !== order) {
    estimated = estimateHeights(cards);
    setEstimate({ count, order, heights: estimated });
  }

  // Moves glide only from the frame after every card was first drawn at
  // its measured height: turning the transition on in that same frame would
  // slide the cards up from where they sat before they were measured.
  const allMeasured =
    count > 0 && [...ids, CREATE_CARD_KEY].every((key) => key in heights);
  if (count === 0 && ready) setReady(false);
  useEffect(() => {
    if (!allMeasured || ready) return;
    const frame = requestAnimationFrame(() => setReady(true));
    return () => cancelAnimationFrame(frame);
  }, [allMeasured, ready]);

  const { columns, createColumn } = placeColumns(
    ids,
    estimated,
    Math.max(1, count),
  );
  columns[createColumn].push(CREATE_CARD_KEY);

  const columnWidth = (width - GAP * (columns.length - 1)) / columns.length;
  const positions: Record<string, { x: number; y: number; width: number }> = {};
  const bottoms = columns.map((keys, column) => {
    let y = 0;
    for (const key of keys) {
      positions[key] = {
        x: column * (columnWidth + GAP),
        y,
        width: columnWidth,
      };
      y += (heights[key] ?? 0) + GAP;
    }
    return y;
  });

  return {
    count,
    positions,
    height: Math.max(0, ...bottoms) - GAP,
    measured: (key: string) => key in heights,
    ready,
    measure,
  };
}

function estimateHeights(cards: CardType[]) {
  return Object.fromEntries(
    cards.map((card) => [card.id, estimateHeight(card)]),
  );
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

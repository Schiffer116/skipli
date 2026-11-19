// The horizontal offset that keeps a dragged favorite inside the bar: its
// edges may not pass the bar's. Rects are client coordinates; x is the offset.
export function clampToBar(
  x: number,
  bar: { left: number; right: number },
  dragged: { left: number; right: number },
): number {
  const min = bar.left - dragged.left;
  const max = bar.right - dragged.right;
  return Math.min(Math.max(x, min), max);
}

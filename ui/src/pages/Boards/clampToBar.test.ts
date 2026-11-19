import { describe, expect, it } from "vitest";

import { clampToBar } from "./clampToBar";

const bar = { left: 0, right: 500 };
const dragged = { left: 100, right: 200 };

describe("clampToBar", () => {
  it("leaves an offset that keeps the favorite inside the bar", () => {
    expect(clampToBar(50, bar, dragged)).toBe(50);
    expect(clampToBar(-100, bar, dragged)).toBe(-100);
    expect(clampToBar(300, bar, dragged)).toBe(300);
  });

  it("stops the favorite at the bar's left edge", () => {
    expect(clampToBar(-400, bar, dragged)).toBe(-100);
  });

  it("stops the favorite at the bar's right edge", () => {
    expect(clampToBar(400, bar, dragged)).toBe(300);
  });
});

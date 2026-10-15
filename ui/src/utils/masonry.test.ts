import { describe, expect, it } from "vitest";

import { placeColumns } from "./masonry";

// Heights by id, in task rows.
const place = (heights: Record<string, number>, count: number) =>
  placeColumns(Object.keys(heights), heights, count);

describe("placeColumns", () => {
  it("fills the top row first, left to right", () => {
    expect(place({ a: 3, b: 3, c: 3 }, 3).columns).toEqual([
      ["a"],
      ["b"],
      ["c"],
    ]);
  });

  it("puts each next card under the shortest column", () => {
    expect(place({ a: 9, b: 3, c: 4, d: 3, e: 3 }, 3).columns).toEqual([
      ["a"],
      ["b", "d"],
      ["c", "e"],
    ]);
  });

  it("breaks ties to the left", () => {
    expect(place({ a: 4, b: 3, c: 3, d: 2 }, 2).columns).toEqual([
      ["a", "d"],
      ["b", "c"],
    ]);
  });

  it("puts the Create button under the shortest column", () => {
    expect(place({ a: 5, b: 3, c: 3 }, 2).createColumn).toBe(0);
    expect(place({ a: 3, b: 5 }, 2).createColumn).toBe(0);
    expect(place({ a: 5, b: 3 }, 2).createColumn).toBe(1);
  });

  it("leaves columns empty when there are too few cards", () => {
    expect(place({ a: 3 }, 3)).toEqual({
      columns: [["a"], [], []],
      createColumn: 1,
    });
  });
});

import { describe, expect, it } from "vitest";

import type { CardType } from "@/pages/Boards/Board/Card";

import { apply } from "./apply";
import { sortMoves } from "./sort";

const card = (...taskIds: string[]): CardType => ({
  id: "c1",
  name: "c1",
  description: "",
  tasks: taskIds.map((id) => ({
    id,
    cardId: "c1",
    name: id,
    description: "",
    status: "pending",
  })),
});

// The card's order after making the moves that sort current into target.
function sorted(current: string[], target: string[]) {
  const moves = sortMoves("b1", "c1", current, target);
  const cards = moves.reduce(apply, [card(...current)]);
  return { order: cards[0].tasks.map((task) => task.id), moves };
}

function* permutations(ids: string[]): Generator<string[]> {
  if (ids.length <= 1) {
    yield ids;
    return;
  }
  for (const [i, id] of ids.entries()) {
    for (const rest of permutations(ids.filter((_, j) => j !== i))) {
      yield [id, ...rest];
    }
  }
}

describe("sortMoves", () => {
  it("makes no moves for a card already in order", () => {
    expect(sortMoves("b1", "c1", ["a", "b", "c"], ["a", "b", "c"])).toEqual([]);
  });

  it("moves only the tasks out of place", () => {
    const { order, moves } = sorted(["a", "b", "c", "d"], ["d", "a", "b", "c"]);
    expect(order).toEqual(["d", "a", "b", "c"]);
    expect(moves).toEqual([
      {
        type: "moveTask",
        boardId: "b1",
        cardId: "c1",
        taskId: "d",
        newCardId: "c1",
        beforeId: null,
        afterId: "a",
      },
    ]);
  });

  it("reaches every order of five tasks in the fewest moves", () => {
    const current = ["a", "b", "c", "d", "e"];
    for (const target of permutations(current)) {
      const { order, moves } = sorted(current, target);
      expect(order).toEqual(target);
      // A reversal keeps one task in place.
      expect(moves.length).toBeLessThanOrEqual(current.length - 1);
    }
    expect(sorted(current, ["e", "d", "c", "b", "a"]).moves).toHaveLength(4);
  });

  it("leaves tasks added meanwhile alone and skips deleted ones", () => {
    // x was added and gone was deleted while Jev answered.
    const { order } = sorted(["a", "x", "b", "c"], ["c", "gone", "b", "a"]);
    expect(order.filter((id) => id !== "x")).toEqual(["c", "b", "a"]);
    expect(order).toContain("x");
  });

  it("undoes a sort with the moves back to the old order", () => {
    const before = ["a", "b", "c", "d"];
    const after = sorted(before, ["c", "a", "d", "b"]).order;
    expect(sorted(after, before).order).toEqual(before);
  });
});

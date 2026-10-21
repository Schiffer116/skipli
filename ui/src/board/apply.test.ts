import { describe, expect, it } from "vitest";

import type { BoardChange } from "@/offline/changes";
import type { CardType } from "@/pages/Boards/Board/Card";

import { apply } from "./apply";

const task = (id: string, cardId: string, status = "pending") => ({
  id,
  cardId,
  name: id,
  description: "",
  status,
});

const card = (id: string, ...taskIds: string[]): CardType => ({
  id,
  name: id,
  description: "",
  tasks: taskIds.map((taskId) => task(taskId, id)),
});

const board = () => [card("c1", "t1", "t2", "t3"), card("c2"), card("c3")];

const ids = (cards: CardType[]) =>
  cards.map((card) => [card.id, ...card.tasks.map((task) => task.id)]);

const moveCard = (
  cardId: string,
  beforeId: string | null,
  afterId: string | null,
): BoardChange => ({
  type: "moveCard",
  boardId: "b1",
  cardId,
  beforeId,
  afterId,
});

const moveTask = (
  taskId: string,
  newCardId: string,
  beforeId: string | null,
  afterId: string | null,
): BoardChange => ({
  type: "moveTask",
  boardId: "b1",
  cardId: "c1",
  taskId,
  newCardId,
  beforeId,
  afterId,
});

describe("apply", () => {
  it("moves a card next to the cards it was dropped between", () => {
    expect(ids(apply(board(), moveCard("c3", "c1", "c2")))).toEqual([
      ["c1", "t1", "t2", "t3"],
      ["c3"],
      ["c2"],
    ]);
    expect(ids(apply(board(), moveCard("c1", null, null)))).toEqual([
      ["c2"],
      ["c3"],
      ["c1", "t1", "t2", "t3"],
    ]);
    expect(ids(apply(board(), moveCard("c3", null, "c1")))).toEqual([
      ["c3"],
      ["c1", "t1", "t2", "t3"],
      ["c2"],
    ]);
  });

  it("moves a task within its card and into another card", () => {
    expect(ids(apply(board(), moveTask("t3", "c1", null, "t1")))).toEqual([
      ["c1", "t3", "t1", "t2"],
      ["c2"],
      ["c3"],
    ]);

    const moved = apply(board(), moveTask("t2", "c2", null, null));
    expect(ids(moved)).toEqual([["c1", "t1", "t3"], ["c2", "t2"], ["c3"]]);
    expect(moved[1].tasks[0].cardId).toBe("c2");
  });

  it("cleans up done tasks only", () => {
    const cards = [
      {
        ...card("c1"),
        tasks: [task("t1", "c1", "done"), task("t2", "c1")],
      },
      card("c2"),
    ];
    const cleaned = apply(cards, { type: "cleanUp", boardId: "b1" });
    expect(ids(cleaned)).toEqual([["c1", "t2"], ["c2"]]);
    // Cards without done tasks are left as they are.
    expect(cleaned[1]).toBe(cards[1]);
  });

  it("creates, edits and deletes cards and tasks", () => {
    let cards = apply([], {
      type: "createCard",
      boardId: "b1",
      cardId: "c1",
      name: "Todo",
      createdAt: "2026-10-20T00:00:00Z",
    });
    cards = apply(cards, {
      type: "createTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
      name: "Milk",
    });
    cards = apply(cards, {
      type: "updateTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
      name: "Oat milk",
      description: "",
      status: "done",
    });
    expect(cards).toEqual([
      {
        id: "c1",
        name: "Todo",
        description: "",
        tasks: [
          {
            id: "t1",
            cardId: "c1",
            name: "Oat milk",
            description: "",
            status: "done",
          },
        ],
      },
    ]);

    cards = apply(cards, {
      type: "deleteTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
    });
    expect(ids(cards)).toEqual([["c1"]]);
    expect(
      apply(cards, { type: "deleteCard", boardId: "b1", cardId: "c1" }),
    ).toEqual([]);
  });

  it("changes nothing more when a change comes twice", () => {
    const changes: BoardChange[] = [
      {
        type: "createCard",
        boardId: "b1",
        cardId: "c4",
        name: "c4",
        createdAt: "2026-10-20T00:00:00Z",
      },
      {
        type: "createTask",
        boardId: "b1",
        cardId: "c2",
        taskId: "t4",
        name: "t4",
      },
      moveCard("c1", "c3", null),
      moveTask("t1", "c2", "t4", null),
      { type: "deleteTask", boardId: "b1", cardId: "c1", taskId: "t2" },
    ];
    for (const change of changes) {
      const once = apply(board(), change);
      expect(apply(once, change)).toEqual(once);
    }
  });

  it("ignores a change to something that isn't there", () => {
    const changes: BoardChange[] = [
      { type: "deleteCard", boardId: "b1", cardId: "gone" },
      { type: "deleteTask", boardId: "b1", cardId: "c1", taskId: "gone" },
      {
        type: "updateCard",
        boardId: "b1",
        cardId: "gone",
        name: "x",
        description: "",
      },
      {
        type: "updateTask",
        boardId: "b1",
        cardId: "c1",
        taskId: "gone",
        name: "x",
        description: "",
        status: "done",
      },
      {
        type: "createTask",
        boardId: "b1",
        cardId: "gone",
        taskId: "t9",
        name: "x",
      },
      moveCard("gone", "c1", null),
      moveTask("gone", "c2", null, null),
      moveTask("t1", "gone", null, null),
    ];
    for (const change of changes) {
      expect(ids(apply(board(), change))).toEqual(ids(board()));
    }
  });
});

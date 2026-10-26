import { describe, expect, it } from "vitest";

import type { BoardChange } from "@/offline/changes";
import type { CardType } from "@/pages/Boards/Board/Card";

import table from "../../../api/internal/boardchange/testdata/board-changes.json";

import { apply, applyOrDrop } from "./apply";

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

type TableCard = Omit<CardType, "tasks"> & {
  tasks: Omit<CardType["tasks"][number], "cardId">[];
};

const withCardIds = (cards: TableCard[]): CardType[] =>
  cards.map((card) => ({
    ...card,
    tasks: card.tasks.map((task) => ({ ...task, cardId: card.id })),
  }));

// The same cases the server passes (api/internal/boardchange/table_test.go).
describe("board-changes.json", () => {
  const { board, cases } = table as unknown as {
    board: TableCard[];
    cases: {
      name: string;
      before?: TableCard[];
      change: Omit<BoardChange, "boardId">;
      after: TableCard[] | "dropped";
    }[];
  };
  it.each(cases)("$name", ({ before, change, after }) => {
    const result = applyOrDrop(withCardIds(before ?? board), {
      ...change,
      boardId: "b1",
    } as BoardChange);
    expect(result).toEqual(after === "dropped" ? after : withCardIds(after));
  });
});

describe("apply", () => {
  it("leaves cards a change doesn't touch as they are", () => {
    const cards = board();
    const cleaned = apply(cards, {
      type: "cleanUp",
      boardId: "b1",
      taskIds: ["t1"],
    });
    expect(ids(cleaned)).toEqual([["c1", "t2", "t3"], ["c2"], ["c3"]]);
    expect(cleaned[1]).toBe(cards[1]);
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
      },
      {
        type: "updateTask",
        boardId: "b1",
        cardId: "c1",
        taskId: "gone",
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

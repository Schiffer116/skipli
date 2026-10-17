import { describe, expect, it } from "vitest";

import type { Board } from "@/pages/Boards/BoardList/BoardList";

import { alreadySaved, applyToList, toRequest, touchedIds } from "./changes";

const board = (id: string, rest: Partial<Board> = {}): Board => ({
  id,
  name: id,
  description: "",
  owner: "me",
  favorite: false,
  taskCount: 0,
  doneCount: 0,
  ...rest,
});

describe("toRequest", () => {
  it("sends the app's own ID with a create", () => {
    const { url, init } = toRequest({
      type: "createTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
      name: "Buy milk",
    });
    expect(url).toBe("/api/boards/b1/cards/c1/tasks");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      id: "t1",
      name: "Buy milk",
    });
  });

  it("moves a task from the card it is in on the server", () => {
    const { url, init } = toRequest({
      type: "moveTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
      newCardId: "c2",
      beforeId: "t0",
      afterId: null,
    });
    expect(url).toBe("/api/boards/b1/cards/c1/tasks/t1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body as string)).toEqual({
      newCardId: "c2",
      beforeId: "t0",
      afterId: null,
    });
  });

  it("deletes without a body", () => {
    expect(toRequest({ type: "deleteBoard", boardId: "b1" })).toEqual({
      url: "/api/boards/b1",
      init: { method: "DELETE" },
    });
  });
});

describe("touchedIds", () => {
  it("marks the board and the card or task a change is about", () => {
    expect(
      touchedIds({ type: "deleteCard", boardId: "b1", cardId: "c1" }),
    ).toEqual(["b1", "c1"]);
    expect(touchedIds({ type: "orderBoards", boardIds: ["b1"] })).toEqual([]);
  });
});

describe("alreadySaved", () => {
  it("counts a 409 on a create as saved by an earlier attempt", () => {
    expect(
      alreadySaved(
        { type: "createBoard", boardId: "b1", name: "", description: "" },
        409,
      ),
    ).toBe(true);
    expect(alreadySaved({ type: "deleteBoard", boardId: "b1" }, 409)).toBe(
      false,
    );
  });
});

describe("applyToList", () => {
  const list = {
    boards: [board("a"), board("b"), board("c")],
    teamBoards: [board("t", { owner: "them" })],
  };

  it("adds a created board as the person's own", () => {
    const next = applyToList(
      list,
      { type: "createBoard", boardId: "n", name: "New", description: "d" },
      "me",
    );
    expect(next.boards.at(-1)).toEqual(
      board("n", { name: "New", description: "d" }),
    );
  });

  it("renames, favorites and deletes in either section", () => {
    let next = applyToList(
      list,
      { type: "updateBoard", boardId: "t", name: "T", description: "x" },
      "me",
    );
    next = applyToList(
      next,
      { type: "favoriteBoard", boardId: "t", favorite: true },
      "me",
    );
    expect(next.teamBoards[0]).toMatchObject({
      name: "T",
      description: "x",
      favorite: true,
    });
    next = applyToList(next, { type: "deleteBoard", boardId: "b" }, "me");
    expect(next.boards.map((b) => b.id)).toEqual(["a", "c"]);
  });

  it("reorders the section the order is for, leaving unnamed boards last", () => {
    const next = applyToList(
      list,
      { type: "orderBoards", boardIds: ["c", "a"] },
      "me",
    );
    expect(next.boards.map((b) => b.id)).toEqual(["c", "a", "b"]);
    expect(next.teamBoards).toEqual(list.teamBoards);
  });
});

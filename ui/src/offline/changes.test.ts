import { describe, expect, it } from "vitest";

import type { Board } from "@/pages/Boards/BoardList/BoardList";

import {
  alreadySaved,
  applyToList,
  isBoardChange,
  toRequest,
  touchedIds,
} from "./changes";

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
      type: "createBoard",
      boardId: "b1",
      name: "Trip",
      description: "",
    });
    expect(url).toBe("/api/boards");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      id: "b1",
      name: "Trip",
      description: "",
    });
  });

  it("deletes without a body", () => {
    expect(toRequest({ type: "deleteBoard", boardId: "b1" })).toEqual({
      url: "/api/boards/b1",
      init: { method: "DELETE" },
    });
  });
});

describe("isBoardChange", () => {
  it("tells changes inside a board from changes to the board list", () => {
    expect(isBoardChange({ type: "cleanUp", boardId: "b1" })).toBe(true);
    expect(isBoardChange({ type: "deleteBoard", boardId: "b1" })).toBe(false);
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

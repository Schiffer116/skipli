import "fake-indexeddb/auto";

import { afterEach, describe, expect, it } from "vitest";

import {
  applyChange,
  deleteCopy,
  loadBoard,
  loadBoards,
  loadList,
  saveBoard,
  saveList,
} from "./copy";

const me = { id: "u1", email: "me@example.com", name: "Me" };
const other = { id: "u2", email: "you@example.com", name: "You" };

const listed = (id: string) => ({
  id,
  name: id,
  description: "",
  owner: me.id,
  favorite: false,
  taskCount: 0,
  doneCount: 0,
});

const saved = (id: string) => ({
  id,
  name: id,
  description: "",
  owner: me.id,
  members: [me],
  cards: [],
});

afterEach(async () => {
  await deleteCopy(me.id);
  await deleteCopy(other.id);
});

describe("saveList", () => {
  it("drops saved boards that left the list", async () => {
    await saveBoard(me.id, saved("kept"));
    await saveBoard(me.id, saved("deleted"));

    await saveList(me.id, { boards: [listed("kept")], teamBoards: [] });

    expect((await loadBoards(me.id)).map((b) => b.id)).toEqual(["kept"]);
  });
});

describe("applyChange", () => {
  it("saves a board made offline as a Solo board", async () => {
    await saveList(me.id, { boards: [], teamBoards: [] });

    await applyChange(me, {
      type: "createBoard",
      boardId: "b1",
      name: "Trip",
      description: "",
    });

    expect(await loadBoard(me.id, "b1")).toMatchObject({
      name: "Trip",
      owner: me.id,
      members: [me],
      cards: [],
    });
    expect((await loadList(me.id))?.boards.map((b) => b.id)).toEqual(["b1"]);
  });

  it("renames and deletes the saved board too", async () => {
    await saveList(me.id, { boards: [listed("b1")], teamBoards: [] });
    await saveBoard(me.id, saved("b1"));

    await applyChange(me, {
      type: "updateBoard",
      boardId: "b1",
      name: "Renamed",
      description: "d",
    });
    expect(await loadBoard(me.id, "b1")).toMatchObject({
      name: "Renamed",
      description: "d",
    });

    await applyChange(me, { type: "deleteBoard", boardId: "b1" });
    expect(await loadBoard(me.id, "b1")).toBeUndefined();
    expect((await loadList(me.id))?.boards).toEqual([]);
  });
});

describe("one copy per person", () => {
  it("keeps each person's boards apart, and deleting one leaves the other", async () => {
    await saveBoard(me.id, saved("mine"));
    await saveBoard(other.id, { ...saved("theirs"), owner: other.id });

    expect((await loadBoards(other.id)).map((b) => b.id)).toEqual(["theirs"]);

    await deleteCopy(me.id);
    expect(await loadBoards(me.id)).toEqual([]);
    expect((await loadBoards(other.id)).map((b) => b.id)).toEqual(["theirs"]);
  });
});

import "fake-indexeddb/auto";

import { afterEach, describe, expect, it } from "vitest";

import type { Change } from "./changes";

import { openDB } from "idb";

import {
  deleteCopy,
  loadBoard,
  loadBoards,
  loadList,
  loadPending,
  pendingStore,
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

const store = pendingStore(me);
let ids = 0;
const queue = (change: Change) => store.queue(change, `change${++ids}`);

describe("queueing a change", () => {
  it("keeps it under its change id", async () => {
    await queue({ type: "deleteBoard", boardId: "b1" });
    expect(await loadPending(me.id)).toMatchObject([
      { id: `change${ids}`, change: { type: "deleteBoard", boardId: "b1" } },
    ]);
  });

  it("saves a board made offline as a Solo board", async () => {
    await saveList(me.id, { boards: [], teamBoards: [] });

    await queue({
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

    await queue({
      type: "updateBoard",
      boardId: "b1",
      name: "Renamed",
      description: "d",
    });
    expect(await loadBoard(me.id, "b1")).toMatchObject({
      name: "Renamed",
      description: "d",
    });

    await queue({ type: "deleteBoard", boardId: "b1" });
    expect(await loadBoard(me.id, "b1")).toBeUndefined();
    expect((await loadList(me.id))?.boards).toEqual([]);
  });

  it("shows a Board change on the saved board", async () => {
    await saveBoard(me.id, {
      ...saved("b1"),
      cards: [{ id: "c1", name: "Todo", description: "", tasks: [] }],
    });
    await queue({
      type: "createTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
      name: "Ship",
    });
    expect(
      (await loadBoard(me.id, "b1"))?.cards[0].tasks.map((t) => t.name),
    ).toEqual(["Ship"]);
  });
});

describe("dropping a change", () => {
  it("discards it and marks the board it touched out of date", async () => {
    await saveList(me.id, { boards: [listed("b1")], teamBoards: [] });
    await saveBoard(me.id, saved("b1"));
    await queue({ type: "deleteCard", boardId: "b1", cardId: "c1" });

    await store.drop((await loadPending(me.id))[0]);

    expect(await loadPending(me.id)).toEqual([]);
    expect(await loadBoard(me.id, "b1")).toMatchObject({ outOfDate: true });
    expect(await loadList(me.id)).not.toHaveProperty("outOfDate");
  });

  it("marks the list out of date for a change to it", async () => {
    await saveList(me.id, { boards: [listed("b1")], teamBoards: [] });
    await queue({ type: "favoriteBoard", boardId: "b1", favorite: true });

    await store.drop((await loadPending(me.id))[0]);

    expect(await loadList(me.id)).toMatchObject({ outOfDate: true });
  });

  it("is forgotten once the board is read from the server again", async () => {
    await saveBoard(me.id, saved("b1"));
    await queue({ type: "deleteCard", boardId: "b1", cardId: "c1" });
    await store.drop((await loadPending(me.id))[0]);

    await saveBoard(me.id, saved("b1"));
    expect(await loadBoard(me.id, "b1")).not.toHaveProperty("outOfDate");
  });
});

describe("an Offline copy from before change ids", () => {
  it("discards refused changes and gives pending ones an id", async () => {
    const old = await openDB(`skipli-offline-${me.id}`, 1, {
      upgrade(db) {
        db.createObjectStore("meta");
        db.createObjectStore("boards", { keyPath: "id" });
        db.createObjectStore("pending", {
          keyPath: "seq",
          autoIncrement: true,
        });
        db.createObjectStore("rejected", {
          keyPath: "seq",
          autoIncrement: true,
        });
      },
    });
    const change = { type: "deleteBoard", boardId: "b1" };
    await old.add("pending", { change, at: 1 });
    await old.add("rejected", { change, status: 404, message: "", at: 1 });
    await old.put("meta", 1, "rejectedSeen");
    old.close();

    const [pending] = await loadPending(me.id);
    expect(pending).toMatchObject({ seq: 1, change });
    expect(pending.id).toMatch(/^[0-9a-f-]{36}$/);

    const upgraded = await openDB(`skipli-offline-${me.id}`);
    expect([...upgraded.objectStoreNames]).toEqual([
      "boards",
      "meta",
      "pending",
    ]);
    expect(await upgraded.get("meta", "rejectedSeen")).toBeUndefined();
    upgraded.close();
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

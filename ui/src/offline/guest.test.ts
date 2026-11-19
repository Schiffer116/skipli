import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Change } from "./changes";

vi.stubGlobal("navigator", { onLine: true });
vi.stubGlobal("window", new EventTarget());
const stored = new Map<string, string>();
vi.stubGlobal("localStorage", {
  getItem: (key: string) => stored.get(key) ?? null,
  setItem: (key: string, value: string) => stored.set(key, value),
  removeItem: (key: string) => stored.delete(key),
});

const { deleteCopy, loadBoards, loadList, loadPending, pendingStore } =
  await import("./copy");
const { GUEST, forAccount, moveGuestInto, startGuestCopy } =
  await import("./guest");
const { guestState, setGuestState } = await import("./session");

const me = { id: "u1", email: "me@example.com", name: "Me" };

const createBoard = (boardId: string): Change => ({
  type: "createBoard",
  boardId,
  name: boardId,
  description: "",
});

// Makes changes as the guest, in order, each under the id `g<n>`.
async function asGuest(...changes: Change[]) {
  const store = pendingStore(GUEST);
  for (const [i, change] of changes.entries()) {
    await store.queue(change, `g${i}`);
  }
}

beforeEach(async () => {
  await startGuestCopy();
  setGuestState("moving");
});

afterEach(async () => {
  await deleteCopy(GUEST.id);
  await deleteCopy(me.id);
  stored.clear();
});

describe("a guest's changes, for an account", () => {
  it("creates the guest's boards first, in the guest's final order", async () => {
    await asGuest(
      createBoard("a"),
      createBoard("b"),
      {
        type: "createCard",
        boardId: "a",
        cardId: "c1",
        name: "c1",
        createdAt: "",
      },
      createBoard("c"),
      { type: "orderBoards", boardIds: ["c", "a", "b"] },
    );

    const changes = forAccount(
      await loadPending(GUEST.id),
      await loadList(GUEST.id),
    );
    expect(changes.map(({ id }) => id)).toEqual(["g3", "g0", "g1", "g2"]);
  });

  it("leaves out boards the guest deleted, and everything done to them", async () => {
    await asGuest(
      createBoard("a"),
      createBoard("gone"),
      { type: "updateBoard", boardId: "gone", name: "Renamed" },
      { type: "favoriteBoard", boardId: "gone", favorite: true },
      { type: "deleteBoard", boardId: "gone" },
    );

    const changes = forAccount(
      await loadPending(GUEST.id),
      await loadList(GUEST.id),
    );
    expect(changes.map(({ change }) => change)).toEqual([createBoard("a")]);
  });

  it("favorites again last, in the bar's order, those still favorites", async () => {
    await asGuest(
      createBoard("a"),
      createBoard("b"),
      createBoard("c"),
      { type: "favoriteBoard", boardId: "a", favorite: true },
      { type: "favoriteBoard", boardId: "b", favorite: true },
      { type: "favoriteBoard", boardId: "c", favorite: true },
      { type: "favoriteBoard", boardId: "a", favorite: false },
      { type: "orderFavorites", boardIds: ["c", "b"] },
    );

    const changes = forAccount(
      await loadPending(GUEST.id),
      await loadList(GUEST.id),
    );
    expect(changes.slice(3)).toEqual([
      {
        id: "g5",
        change: { type: "favoriteBoard", boardId: "c", favorite: true },
      },
      {
        id: "g4",
        change: { type: "favoriteBoard", boardId: "b", favorite: true },
      },
    ]);
  });
});

describe("moving a guest's boards into an account", () => {
  it("queues them as the account's, its own, and deletes the guest's copy", async () => {
    await asGuest(createBoard("a"), {
      type: "createCard",
      boardId: "a",
      cardId: "c1",
      name: "c1",
      createdAt: "",
    });

    await moveGuestInto(me);

    expect((await loadPending(me.id)).map(({ id }) => id)).toEqual([
      "g0",
      "g1",
    ]);
    const [board] = await loadBoards(me.id);
    expect(board).toMatchObject({ id: "a", owner: me.id, members: [me] });
    expect(board.cards.map((card) => card.id)).toEqual(["c1"]);
    expect(guestState()).toBeNull();
    expect(await loadPending(GUEST.id)).toEqual([]);
    expect(await loadList(GUEST.id)).toBeUndefined();
  });

  it("queues each change once when moved again", async () => {
    await asGuest(createBoard("a"));
    await moveGuestInto(me);
    // As if the tab closed before the guest's copy was deleted.
    await startGuestCopy();
    await asGuest(createBoard("a"));
    await moveGuestInto(me);

    expect(await loadPending(me.id)).toHaveLength(1);
  });
});

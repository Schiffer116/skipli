import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LoaderFunctionArgs } from "react-router";

import type { Change } from "@/offline/changes";

const me = { id: "u1", email: "me@example.com", name: "Me" };

const card = (id: string, ...taskIds: string[]) => ({
  id,
  name: id,
  description: "",
  tasks: taskIds.map((taskId) => ({
    id: taskId,
    cardId: id,
    name: taskId,
    description: "",
    status: "pending",
  })),
});

const onServer = {
  id: "b1",
  name: "Board",
  description: "",
  owner: me.id,
  members: [me],
  cards: [card("c1", "t1"), card("c2")],
};

const fetched: string[] = [];
// False: no request gets an answer, as with no connection.
let reachable = true;
let answer: (url: string) => Response = () => Response.json(onServer);

vi.mock("@/utils/apiFetch", () => ({
  apiFetch: async (url: string) => {
    fetched.push(url);
    if (!reachable) throw new TypeError("Failed to fetch");
    if (url === "/api/me") return Response.json(me);
    return answer(url);
  },
}));

// With someone signed in here, opening a board joins its socket early.
vi.mock("@/utils/socket", () => ({
  socket: { joinEarly: () => () => {} },
}));

const network = { onLine: true };
vi.stubGlobal("navigator", network);
vi.stubGlobal("window", new EventTarget());
const stored = new Map<string, string>();
vi.stubGlobal("localStorage", {
  getItem: (key: string) => stored.get(key) ?? null,
  setItem: (key: string, value: string) => stored.set(key, value),
  removeItem: (key: string) => stored.delete(key),
});

const { deleteCopy, loadBoard, pendingStore, saveBoard, saveMe } =
  await import("@/offline/copy");
const { stopPending } = await import("@/offline/pending");
const addPending = (change: Change) =>
  pendingStore(me).queue(change, `change-${change.type}`);
const { setActiveUser } = await import("@/offline/session");
const { boardListLoader, boardsLoader, boardViewLoader, forgetMe } =
  await import("./loaders");
const { fetchBoard } = await import("@/board/fetch");

const load = () =>
  boardViewLoader({
    params: { boardId: "b1" },
  } as unknown as LoaderFunctionArgs);

const saved = { ...onServer, name: "Old name", cards: [card("c1")] };

const boardReads = () => fetched.filter((url) => url.startsWith("/api/boards"));

beforeEach(() => {
  fetched.length = 0;
  network.onLine = true;
  reachable = true;
  answer = () => Response.json(onServer);
});

afterEach(async () => {
  stopPending();
  forgetMe();
  setActiveUser(null);
  await deleteCopy(me.id);
});

describe("opening a board", () => {
  it("with no Offline copy, shows it from one request", async () => {
    const board = await load();
    expect(board.name).toBe("Board");
    expect(board.cardsWithTasks).toEqual(onServer.cards);
    expect(boardReads()).toEqual(["/api/boards/b1"]);
    expect((await loadBoard(me.id, "b1"))?.cards).toEqual(onServer.cards);
  });

  it("with an Offline copy, shows it without asking the server", async () => {
    await saveBoard(me.id, saved);

    const board = await load();
    expect(board.name).toBe("Old name");
    expect(board.cardsWithTasks).toEqual(saved.cards);
    expect(boardReads()).toEqual([]);
  });

  it("lays Pending changes on top of the server's board", async () => {
    await addPending({
      type: "createTask",
      boardId: "b1",
      cardId: "c2",
      taskId: "t2",
      name: "t2",
    });
    await addPending({
      type: "updateBoard",
      boardId: "b1",
      name: "Renamed offline",
      description: "",
    });
    // The server can't take them yet.
    answer = (url) =>
      url === "/api/boards/b1"
        ? Response.json(onServer)
        : new Response(null, { status: 503 });

    expect(await load()).toMatchObject({
      name: "Renamed offline",
      cardsWithTasks: [card("c1", "t1"), card("c2", "t2")],
    });
  });

  it.each([403, 404])(
    "forgets a board the server answers %i for",
    async (status) => {
      // As when a copy's board loads again once its socket opens.
      await saveBoard(me.id, saved);
      answer = () => new Response(null, { status });

      await expect(fetchBoard(me.id, "b1")).rejects.toBeDefined();
      expect(await loadBoard(me.id, "b1")).toBeUndefined();
    },
  );
});

// Phones often say they're online with no connection to speak of.
describe("online with no answer from the server", () => {
  beforeEach(async () => {
    setActiveUser(me.id);
    await saveMe(me);
    reachable = false;
  });

  it("opens a board from the Offline copy", async () => {
    await saveBoard(me.id, saved);
    expect(await load()).toMatchObject({ name: "Old name" });
  });

  it("says a board not on this device isn't available offline", async () => {
    await expect(load()).rejects.toMatchObject({
      init: { status: 503 },
      data: "board",
    });
  });

  it("lists the boards on this device", async () => {
    await saveBoard(me.id, saved);
    expect((await boardsLoader()).me).toEqual(me);
    expect(await boardListLoader()).toMatchObject({
      offline: { missing: false },
    });
  });

  it("still shows a server error as one", async () => {
    reachable = true;
    answer = () => new Response(null, { status: 500 });
    await expect(boardListLoader()).rejects.toBeDefined();
  });
});

describe("favoritesOf", () => {
  const board = (id: string, rest = {}) => ({
    id,
    name: id,
    description: "",
    owner: me.id,
    favorite: true,
    taskCount: 0,
    doneCount: 0,
    ...rest,
  });

  it("sorts by favorite order, across owned and team boards", async () => {
    const { favoritesOf } = await import("./loaders");
    const favorites = favoritesOf({
      boards: [
        board("a", { favoriteOrder: 3 }),
        board("b", { favoriteOrder: 1 }),
        board("x", { favorite: false }),
      ],
      teamBoards: [board("t", { favoriteOrder: 2 })],
    });
    expect(favorites.map((f) => f.id)).toEqual(["b", "t", "a"]);
  });

  it("keeps board list order for favorites with no order, ahead of ordered ones", async () => {
    const { favoritesOf } = await import("./loaders");
    const favorites = favoritesOf({
      boards: [board("a", { favoriteOrder: 5 }), board("b"), board("c")],
      teamBoards: [board("t")],
    });
    expect(favorites.map((f) => f.id)).toEqual(["b", "c", "t", "a"]);
  });
});

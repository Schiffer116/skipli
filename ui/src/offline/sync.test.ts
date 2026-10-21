import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Change } from "./changes";

const fetches: { url: string; init: RequestInit }[] = [];
let answer: (url: string) => number = () => 200;

vi.mock("@/utils/apiFetch", () => ({
  apiFetch: async (url: string, init: RequestInit) => {
    fetches.push({ url, init });
    const status = answer(url);
    return new Response(status === 204 ? null : `status ${status}`, {
      status,
    });
  },
}));

const network = { onLine: true };
vi.stubGlobal("navigator", network);
vi.stubGlobal("window", new EventTarget());

const { deleteCopy, loadPending, loadRejected, saveList, loadList } =
  await import("./copy");
const { startSync, stopSync, submit, sync } = await import("./sync");

const me = { id: "u1", email: "me@example.com", name: "Me" };

const goOffline = () => {
  network.onLine = false;
  window.dispatchEvent(new Event("offline"));
};
const goOnline = () => {
  network.onLine = true;
  window.dispatchEvent(new Event("online"));
};

const createTask = (taskId: string): Change => ({
  type: "createTask",
  boardId: "b1",
  cardId: "c1",
  taskId,
  name: taskId,
});

beforeEach(async () => {
  fetches.length = 0;
  answer = () => 200;
  network.onLine = true;
  await startSync(me);
});

afterEach(async () => {
  stopSync();
  await deleteCopy(me.id);
});

describe("submit", () => {
  it("sends right away when online", async () => {
    expect(await submit(createTask("t1"))).toBe(true);
    expect(fetches.map((f) => f.url)).toEqual(["/api/boards/b1/changes"]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("sends each change under an id of its own for the server's broadcast", async () => {
    await submit(createTask("t1"));
    await submit(createTask("t2"));
    const ids = fetches.map(
      (f) => (f.init.headers as Record<string, string>)["X-Change-Id"],
    );
    expect(ids[0]).toBeTruthy();
    expect(ids[1]).not.toBe(ids[0]);
  });

  it("reports an online send the server refuses as not made", async () => {
    answer = () => 404;
    expect(await submit(createTask("t1"))).toBe(false);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("keeps changes made offline and applies them to the saved list", async () => {
    await saveList(me.id, { boards: [], teamBoards: [] });
    goOffline();

    expect(
      await submit({
        type: "createBoard",
        boardId: "b9",
        name: "Trip",
        description: "",
      }),
    ).toBe(true);

    expect(fetches).toEqual([]);
    expect((await loadPending(me.id)).map((p) => p.change.type)).toEqual([
      "createBoard",
    ]);
    expect((await loadList(me.id))?.boards.map((b) => b.name)).toEqual([
      "Trip",
    ]);
  });
});

describe("sync", () => {
  it("sends pending changes in the order they were made once online", async () => {
    goOffline();
    for (const id of ["t1", "t2", "t3"]) await submit(createTask(id));

    goOnline();
    await sync();

    expect(
      fetches.map((f) => JSON.parse(f.init.body as string).taskId),
    ).toEqual(["t1", "t2", "t3"]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("keeps what the server refuses as rejected, and sends the rest", async () => {
    goOffline();
    for (const id of ["t1", "t2", "t3"]) await submit(createTask(id));
    answer = () =>
      fetches.length === 2 ? 404 : fetches.length === 3 ? 409 : 201;

    goOnline();
    await sync();

    expect(await loadPending(me.id)).toEqual([]);
    const rejected = await loadRejected(me.id);
    expect(rejected.map((r) => [r.change, r.status])).toEqual([
      [createTask("t2"), 404],
    ]);
  });

  it("stops on a server error and tries again later, in order", async () => {
    goOffline();
    for (const id of ["t1", "t2"]) await submit(createTask(id));
    answer = () => 503;

    goOnline();
    await sync();
    expect(fetches).toHaveLength(1);
    expect(await loadPending(me.id)).toHaveLength(2);

    answer = () => 201;
    await sync();
    expect(await loadPending(me.id)).toEqual([]);
    expect(await loadRejected(me.id)).toEqual([]);
  });

  it("tries again on its own after a network failure", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    try {
      goOffline();
      await submit(createTask("t1"));
      answer = () => 503;
      goOnline();
      await sync();
      expect(await loadPending(me.id)).toHaveLength(1);

      answer = () => 201;
      await vi.advanceTimersByTimeAsync(5_000);
      await vi.waitFor(async () =>
        expect(await loadPending(me.id)).toEqual([]),
      );
      expect(fetches).toHaveLength(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it("sends a change made while syncing after the ones before it", async () => {
    goOffline();
    await submit(createTask("t1"));
    network.onLine = true;

    const running = sync();
    await submit(createTask("t2"));
    await running;
    await sync();

    expect(
      fetches.map((f) => JSON.parse(f.init.body as string).taskId),
    ).toEqual(["t1", "t2"]);
  });
});

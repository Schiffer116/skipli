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

const fake = await vi.hoisted(async () =>
  (await import("@/utils/socket.fake")).fakeSocket(),
);
vi.mock("@/utils/socket", () => ({ socket: fake.socket }));

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

const sentIds = () =>
  fake.sent.map((change) => ("taskId" in change ? change.taskId : null));

beforeEach(async () => {
  fetches.length = 0;
  answer = () => 200;
  fake.reset();
  fake.open.add("b1");
  network.onLine = true;
  await startSync(me);
});

afterEach(async () => {
  stopSync();
  await deleteCopy(me.id);
});

describe("submit", () => {
  it("sends a Board change over its board's socket right away", async () => {
    expect(await submit(createTask("t1"))).toBe(true);
    expect(sentIds()).toEqual(["t1"]);
    expect(fetches).toEqual([]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("sends a change to the board list over HTTP", async () => {
    expect(await submit({ type: "deleteBoard", boardId: "b1" })).toBe(true);
    expect(fetches.map((f) => [f.init.method, f.url])).toEqual([
      ["DELETE", "/api/boards/b1"],
    ]);
    expect(fake.sent).toEqual([]);
  });

  it("reports an online send the server refuses as not made", async () => {
    fake.status = () => 404;
    expect(await submit(createTask("t1"))).toBe(false);
    answer = () => 403;
    expect(await submit({ type: "deleteBoard", boardId: "b1" })).toBe(false);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("counts a 409 on a create as saved by an earlier attempt", async () => {
    fake.status = () => 409;
    expect(await submit(createTask("t1"))).toBe(true);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("keeps a change made before the board's socket opened, and sends it once it opens", async () => {
    fake.open.clear();
    expect(await submit(createTask("t1"))).toBe(true);
    expect(fake.sent).toEqual([]);
    expect(await loadPending(me.id)).toHaveLength(1);

    fake.open.clear();
    fake.connect("b1");
    await vi.waitFor(async () => expect(await loadPending(me.id)).toEqual([]));
    expect(sentIds()).toEqual(["t1"]);
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

    expect(sentIds()).toEqual(["t1", "t2", "t3"]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("keeps what the server refuses as rejected, and sends the rest", async () => {
    goOffline();
    for (const id of ["t1", "t2", "t3"]) await submit(createTask(id));
    let sends = 0;
    fake.status = () => (++sends === 2 ? 404 : sends === 3 ? 409 : 200);

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
    fake.status = () => 503;

    goOnline();
    await sync();
    expect(fake.sent).toHaveLength(1);
    expect(await loadPending(me.id)).toHaveLength(2);

    fake.status = () => 200;
    await sync();
    expect(await loadPending(me.id)).toEqual([]);
    expect(await loadRejected(me.id)).toEqual([]);
  });

  it("tries again on its own after a network failure", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    try {
      goOffline();
      await submit(createTask("t1"));
      fake.status = () => "unreachable";
      goOnline();
      await sync();
      expect(await loadPending(me.id)).toHaveLength(1);

      fake.status = () => 200;
      await vi.advanceTimersByTimeAsync(5_000);
      await vi.waitFor(async () =>
        expect(await loadPending(me.id)).toEqual([]),
      );
      expect(sentIds()).toEqual(["t1"]);
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

    expect(sentIds()).toEqual(["t1", "t2"]);
  });
});

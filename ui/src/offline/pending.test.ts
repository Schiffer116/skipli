import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Change } from "./changes";

const network = { onLine: true };
vi.stubGlobal("navigator", network);
vi.stubGlobal("window", new EventTarget());

const {
  deleteCopy,
  loadBoard,
  loadList,
  loadPending,
  pendingStore,
  saveBoard,
  saveList,
} = await import("./copy");
const { createPending } = await import("./pending");

const me = { id: "u1", email: "me@example.com", name: "Me" };

// A fake server: records what's sent and answers with a status, or with
// nothing at all.
const sent: { change: Change; id: string }[] = [];
let answer: (change: Change) => number | "none" = () => 200;
const transport = async (change: Change, id: string) => {
  sent.push({ change, id });
  const status = answer(change);
  if (status === "none") throw new TypeError("Failed to fetch");
  return new Response(null, { status });
};

const createTask = (taskId: string): Change => ({
  type: "createTask",
  boardId: "b1",
  cardId: "c1",
  taskId,
  name: taskId,
});
const taskIds = () =>
  sent.map(({ change }) => (change as { taskId: string }).taskId);

let pending: ReturnType<typeof createPending>;
const start = () =>
  (pending = createPending({
    userId: me.id,
    transport,
    store: pendingStore(me),
  }));

const goOnline = () => {
  network.onLine = true;
  window.dispatchEvent(new Event("online"));
};

beforeEach(async () => {
  sent.length = 0;
  answer = () => 200;
  network.onLine = true;
  start();
  await pending.settled();
});

afterEach(async () => {
  pending.close();
  vi.useRealTimers();
  await deleteCopy(me.id);
});

describe("what the server answers", () => {
  it.each([
    ["2xx", 201, "accepted"],
    ["409 to a create, saved by an earlier attempt", 409, "accepted"],
    ["no answer", "none" as const, "waiting"],
    ["5xx", 503, "waiting"],
    ["429", 429, "waiting"],
    ["401", 401, "waiting"],
    ["another 4xx", 404, "dropped"],
  ])("%s: %s", async (_, status, outcome) => {
    answer = () => status;
    expect(await pending.make(createTask("t1"))).toBe(outcome);
    expect(await loadPending(me.id)).toHaveLength(
      outcome === "waiting" ? 1 : 0,
    );
  });

  it("drops a 409 to anything but a create", async () => {
    answer = () => 409;
    expect(await pending.make({ type: "deleteBoard", boardId: "b1" })).toBe(
      "dropped",
    );
  });
});

describe("queueing", () => {
  it("keeps the change and shows it in the Offline copy before sending", async () => {
    await saveList(me.id, { boards: [], teamBoards: [] });
    network.onLine = false;

    const made = {
      type: "createBoard",
      boardId: "b9",
      name: "Trip",
      description: "",
    } as const;
    expect(await pending.make(made)).toBe("waiting");

    expect(sent).toEqual([]);
    expect((await loadPending(me.id)).map((p) => p.change)).toEqual([made]);
    expect((await loadList(me.id))?.boards.map((b) => b.name)).toEqual([
      "Trip",
    ]);
    expect(pending.state()).toMatchObject({ queued: 1, waiting: 1 });
    expect(pending.state().waitingIds).toEqual(new Set(["b9"]));
  });

  it("sends changes one at a time, in the order made, across boards and the list", async () => {
    await Promise.all([
      pending.make(createTask("t1")),
      pending.make({ type: "orderBoards", boardIds: ["b2", "b1"] }),
      pending.make({ type: "deleteCard", boardId: "b2", cardId: "c9" }),
    ]);
    expect(sent.map(({ change }) => change.type)).toEqual([
      "createTask",
      "orderBoards",
      "deleteCard",
    ]);
  });

  it("sends what was made offline once online, in order", async () => {
    network.onLine = false;
    for (const id of ["t1", "t2", "t3"]) await pending.make(createTask(id));

    goOnline();
    await pending.settled();

    expect(taskIds()).toEqual(["t1", "t2", "t3"]);
    expect(await loadPending(me.id)).toEqual([]);
    expect(pending.state()).toMatchObject({ queued: 0, waiting: 0 });
  });

  it("sends what an earlier visit left, on starting", async () => {
    network.onLine = false;
    await pending.make(createTask("t1"));
    pending.close();
    network.onLine = true;

    start();
    await vi.waitFor(async () => expect(await loadPending(me.id)).toEqual([]));
    expect(taskIds()).toEqual(["t1"]);
  });

  it("doesn't mark a change on its first way out as waiting", async () => {
    let release = () => {};
    const gate = new Promise<void>((resolve) => (release = resolve));
    const states: number[] = [];
    const slow = async (change: Change, id: string) => {
      await gate;
      return transport(change, id);
    };
    pending.close();
    pending = createPending({
      userId: me.id,
      transport: slow,
      store: pendingStore(me),
    });
    pending.subscribe(() => states.push(pending.state().waiting));

    const made = pending.make(createTask("t1"));
    await vi.waitFor(() => expect(pending.state().sending).toBe(true));
    expect(pending.state()).toMatchObject({ queued: 1, waiting: 0 });
    release();
    expect(await made).toBe("accepted");
    expect(states.every((waiting) => waiting === 0)).toBe(true);
  });
});

describe("a server error", () => {
  it("holds the queue, and later changes wait behind it", async () => {
    answer = () => 503;
    expect(await pending.make(createTask("t1"))).toBe("waiting");
    expect(await pending.make(createTask("t2"))).toBe("waiting");

    expect(taskIds()).toEqual(["t1"]);
    expect(pending.state()).toMatchObject({ queued: 2, waiting: 2 });

    answer = () => 201;
    await pending.send();
    expect(taskIds()).toEqual(["t1", "t1", "t2"]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("tries again on its own, waiting longer each time", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    answer = () => "none";
    await pending.make(createTask("t1"));
    expect(sent).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(5_000);
    await vi.waitFor(() => expect(sent).toHaveLength(2));

    answer = () => 201;
    await vi.advanceTimersByTimeAsync(5_000);
    expect(sent).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(5_000);
    await vi.waitFor(async () => expect(await loadPending(me.id)).toEqual([]));
    expect(sent).toHaveLength(3);
  });

  it("sends every attempt under the change's one id", async () => {
    answer = () => 503;
    await pending.make(createTask("t1"));
    answer = () => 201;
    await pending.send();

    expect(sent).toHaveLength(2);
    expect(sent[0].id).toBeTruthy();
    expect(sent[1].id).toBe(sent[0].id);
  });
});

describe("an expired session", () => {
  it("pauses until the same person signs in again", async () => {
    answer = () => 401;
    await pending.make(createTask("t1"));
    expect(pending.state()).toMatchObject({ paused: true, queued: 1 });

    // Nothing more goes out while paused, even on reconnecting.
    expect(await pending.make(createTask("t2"))).toBe("waiting");
    goOnline();
    await pending.settled();
    expect(taskIds()).toEqual(["t1"]);

    answer = () => 201;
    pending.resume();
    await pending.settled();
    expect(pending.state()).toMatchObject({ paused: false, queued: 0 });
    expect(taskIds()).toEqual(["t1", "t1", "t2"]);
  });
});

describe("a dropped change", () => {
  it("is counted, told of, and the rest are sent", async () => {
    network.onLine = false;
    for (const id of ["t1", "t2", "t3"]) await pending.make(createTask(id));
    answer = (change) =>
      (change as { taskId: string }).taskId === "t3" ? 201 : 404;
    const dropped = vi.fn();
    pending.onDropped(dropped);

    goOnline();
    await pending.settled();

    expect(await loadPending(me.id)).toEqual([]);
    expect(pending.state().dropped).toBe(2);
    expect(dropped.mock.calls).toEqual([
      [createTask("t1")],
      [createTask("t2")],
    ]);

    pending.seenDropped();
    expect(pending.state().dropped).toBe(0);
  });

  it("marks the board it touched out of date", async () => {
    await saveBoard(me.id, {
      id: "b1",
      name: "Board",
      description: "",
      owner: me.id,
      members: [me],
      cards: [],
    });
    answer = () => 404;
    expect(await pending.make(createTask("t1"))).toBe("dropped");
    expect(await loadBoard(me.id, "b1")).toMatchObject({ outOfDate: true });
  });
});

describe("the server's broadcast", () => {
  it("is recognised as this person's own, before and after the answer", async () => {
    let release = () => {};
    const gate = new Promise<void>((resolve) => (release = resolve));
    pending.close();
    pending = createPending({
      userId: me.id,
      transport: async (change, id) => {
        sent.push({ change, id });
        await gate;
        return new Response(null, { status: 201 });
      },
      store: pendingStore(me),
    });

    const made = pending.make(createTask("t1"));
    await vi.waitFor(() => expect(sent).toHaveLength(1));
    // The server broadcasts before it answers.
    expect(pending.isOwn(sent[0].id)).toBe(true);
    release();
    await made;
    expect(pending.isOwn(sent[0].id)).toBe(true);
    expect(pending.isOwn("a teammate's")).toBe(false);
  });
});

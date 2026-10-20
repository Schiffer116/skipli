import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BoardChange } from "@/offline/changes";

// A fake server: records what's sent and answers with a status, or can't
// be reached at all.
const fetches: { url: string; init: RequestInit }[] = [];
let answer: () => number | "unreachable" = () => 200;

vi.mock("@/utils/apiFetch", () => ({
  apiFetch: async (url: string, init: RequestInit) => {
    fetches.push({ url, init });
    const status = answer();
    if (status === "unreachable") throw new TypeError("Failed to fetch");
    return new Response(`status ${status}`, { status });
  },
}));

const network = { onLine: true };
vi.stubGlobal("navigator", network);
vi.stubGlobal("window", new EventTarget());

const { deleteCopy, loadBoard, loadPending, saveBoard } =
  await import("@/offline/copy");
const { setOnRefused, startSync, stopSync, sync } =
  await import("@/offline/sync");
const { openBoard } = await import("./board");

const me = { id: "u1", email: "me@example.com", name: "Me" };

// A fake socket: what teammates would receive, and a way to play back
// what they send.
function fakeSocket() {
  const handlers = new Map<string, (...args: never[]) => void>();
  return {
    sent: [] as unknown[][],
    connect: () => {},
    disconnect: () => {},
    emit(event: string, ...args: unknown[]) {
      this.sent.push([event, ...args]);
    },
    on: (event: string, handler: (...args: never[]) => void) =>
      void handlers.set(event, handler),
    off: (event: string) => void handlers.delete(event),
    receive: (event: string, ...args: unknown[]) =>
      (handlers.get(event) as (...args: unknown[]) => void)?.(...args),
  };
}

const loaded = [{ id: "c1", name: "Todo", description: "", tasks: [] }];

const createTask = (taskId: string): BoardChange => ({
  type: "createTask",
  boardId: "b1",
  cardId: "c1",
  taskId,
  name: taskId,
});

const taskIds = (cards: { tasks: { id: string }[] }[]) =>
  cards.flatMap((card) => card.tasks.map((task) => task.id));

let socket: ReturnType<typeof fakeSocket>;
let board: ReturnType<typeof openBoard>;
let stop: () => void;
const refused = vi.fn();

beforeEach(async () => {
  fetches.length = 0;
  answer = () => 200;
  network.onLine = true;
  refused.mockClear();
  setOnRefused(refused);
  await startSync(me);
  socket = fakeSocket();
  board = openBoard(me.id, "b1", loaded, socket);
  stop = board.start();
});

afterEach(async () => {
  stop();
  stopSync();
  await deleteCopy(me.id);
});

describe("make", () => {
  it("shows the change at once, sends it, then passes it to teammates", async () => {
    const made = board.make(createTask("t1"));
    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(socket.sent).toEqual([]);

    await made;
    expect(fetches.map((f) => f.url)).toEqual([
      "/api/boards/b1/cards/c1/tasks",
    ]);
    expect(socket.sent).toEqual([["change", createTask("t1")]]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("keeps the change as pending offline, and sends it on reconnect", async () => {
    network.onLine = false;
    await board.make(createTask("t1"));

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(fetches).toEqual([]);
    expect(await loadPending(me.id)).toHaveLength(1);

    network.onLine = true;
    await sync();
    expect(await loadPending(me.id)).toEqual([]);
    expect(socket.sent).toEqual([["change", createTask("t1")]]);
  });

  it.each([
    ["can't be reached", "unreachable" as const],
    ["answers with a server error", 503],
  ])("keeps the change as pending when the server %s", async (_, status) => {
    answer = () => status;
    await board.make(createTask("t1"));

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(await loadPending(me.id)).toHaveLength(1);
    expect(socket.sent).toEqual([]);
    expect(refused).not.toHaveBeenCalled();

    // Later changes wait behind it, so they still arrive in order.
    answer = () => 200;
    await board.make(createTask("t2"));
    await sync();
    expect(
      fetches.slice(1).map((f) => JSON.parse(f.init.body as string).id),
    ).toEqual(["t1", "t2"]);
  });

  it("reloads the board and says so when the server refuses", async () => {
    answer = () => 404;
    await board.make(createTask("t1"));

    expect(refused).toHaveBeenCalledOnce();
    expect(await loadPending(me.id)).toEqual([]);
    expect(socket.sent).toEqual([]);
  });

  it("sends changes in the order they were made", async () => {
    await Promise.all(
      ["t1", "t2", "t3"].map((id) => board.make(createTask(id))),
    );
    expect(fetches.map((f) => JSON.parse(f.init.body as string).id)).toEqual([
      "t1",
      "t2",
      "t3",
    ]);
  });
});

describe("teammates' changes", () => {
  it("show on the board they were made on", () => {
    const listener = vi.fn();
    board.subscribe(listener);

    socket.receive("change", createTask("t1"));
    socket.receive("change", { ...createTask("t2"), boardId: "b2" });

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(listener).toHaveBeenCalledOnce();
  });
});

describe("the Offline copy", () => {
  it("keeps what's on screen once the board closes", async () => {
    await saveBoard(me.id, {
      id: "b1",
      name: "Board",
      description: "",
      owner: me.id,
      members: [me],
      cards: loaded,
    });
    socket.receive("change", createTask("t1"));
    stop();

    await vi.waitFor(async () =>
      expect(taskIds((await loadBoard(me.id, "b1"))!.cards)).toEqual(["t1"]),
    );
  });
});

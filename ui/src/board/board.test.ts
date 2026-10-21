import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BoardChange } from "@/offline/changes";
import type { CardType } from "@/pages/Boards/Board/Card";

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

// A fake socket: playing back what teammates saved, and the socket opening.
const socket = await vi.hoisted(async () => {
  const changes = new Set<(change: unknown) => void>();
  const opens = new Set<(boardId: string) => void>();
  const on =
    <T>(handlers: Set<T>) =>
    (handler: T) => {
      handlers.add(handler);
      return () => void handlers.delete(handler);
    };
  return {
    joined: [] as string[],
    receive: (change: unknown) => changes.forEach((handler) => handler(change)),
    open: (boardId: string) => opens.forEach((handler) => handler(boardId)),
    module: {
      join(boardId: string) {
        socket.joined.push(boardId);
        return () =>
          void socket.joined.splice(socket.joined.indexOf(boardId), 1);
      },
      onChange: on(changes),
      onOpen: on(opens),
      newChangeId: () => "id",
    },
  };
});
vi.mock("@/utils/socket", () => ({
  socket: socket.module,
  newChangeId: socket.module.newChangeId,
}));

const loaded: CardType[] = [
  { id: "c1", name: "Todo", description: "", tasks: [] },
];

const createTask = (taskId: string): BoardChange => ({
  type: "createTask",
  boardId: "b1",
  cardId: "c1",
  taskId,
  name: taskId,
});

const task = (id: string) => ({
  id,
  cardId: "c1",
  name: id,
  description: "",
  status: "pending",
});

const taskIds = (cards: { tasks: { id: string }[] }[]) =>
  cards.flatMap((card) => card.tasks.map((task) => task.id));

// What the server has when the board is loaded again.
let serverCards = loaded;
const reload = vi.fn(async () => serverCards);
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
  serverCards = loaded;
  reload.mockClear();
  board = openBoard(me.id, "b1", loaded, reload);
  stop = board.start();
});

afterEach(async () => {
  stop();
  stopSync();
  await deleteCopy(me.id);
});

describe("make", () => {
  it("shows the change at once, then sends it over HTTP", async () => {
    const made = board.make(createTask("t1"));
    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(fetches).toEqual([]);

    await made;
    expect(fetches.map((f) => f.url)).toEqual(["/api/boards/b1/changes"]);
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
    expect(fetches).toHaveLength(1);
  });

  it.each([
    ["can't be reached", "unreachable" as const],
    ["answers with a server error", 503],
  ])("keeps the change as pending when the server %s", async (_, status) => {
    answer = () => status;
    await board.make(createTask("t1"));

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(await loadPending(me.id)).toHaveLength(1);
    expect(refused).not.toHaveBeenCalled();

    // Later changes wait behind it, so they still arrive in order.
    answer = () => 200;
    await board.make(createTask("t2"));
    await sync();
    expect(
      fetches.slice(1).map((f) => JSON.parse(f.init.body as string).taskId),
    ).toEqual(["t1", "t2"]);
  });

  it("reloads the board and says so when the server refuses", async () => {
    answer = () => 404;
    await board.make(createTask("t1"));

    expect(refused).toHaveBeenCalledOnce();
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("sends changes in the order they were made", async () => {
    await Promise.all(
      ["t1", "t2", "t3"].map((id) => board.make(createTask(id))),
    );
    expect(
      fetches.map((f) => JSON.parse(f.init.body as string).taskId),
    ).toEqual(["t1", "t2", "t3"]);
  });
});

describe("teammates' changes", () => {
  it("show on the board they were made on", () => {
    const listener = vi.fn();
    board.subscribe(listener);

    socket.receive(createTask("t1"));
    socket.receive({ ...createTask("t2"), boardId: "b2" });

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(listener).toHaveBeenCalledOnce();
  });
});

describe("when the socket opens", () => {
  const catchUp = () => {
    socket.open("b1");
    return vi.waitFor(() => expect(reload).toHaveBeenCalled());
  };

  it("replaces the board with the server's, which may hold changes missed while it was closed", async () => {
    serverCards = [
      { id: "c1", name: "Todo", description: "", tasks: [task("t9")] },
    ];
    await catchUp();
    await vi.waitFor(() => expect(taskIds(board.cards())).toEqual(["t9"]));
  });

  it("keeps changes the server hasn't accepted yet on top", async () => {
    network.onLine = false;
    await board.make(createTask("t1"));
    network.onLine = true;
    answer = () => 503;
    serverCards = [
      { id: "c1", name: "Todo", description: "", tasks: [task("t9")] },
    ];

    await catchUp();
    await vi.waitFor(() =>
      expect(taskIds(board.cards())).toEqual(["t9", "t1"]),
    );
  });

  it("waits for a change still on its way to the server", async () => {
    let release = () => {};
    answer = () => 200;
    const gate = new Promise<void>((resolve) => (release = resolve));
    reload.mockImplementationOnce(async () => {
      await gate;
      return serverCards;
    });
    // A change made before the socket opened is saved before the reload.
    const made = board.make(createTask("t1"));
    await catchUp();
    expect(fetches).toHaveLength(1);
    serverCards = [
      { id: "c1", name: "Todo", description: "", tasks: [task("t1")] },
    ];
    release();
    await made;
    await vi.waitFor(() => expect(taskIds(board.cards())).toEqual(["t1"]));
  });

  it("loads again when a change was made while it loaded", async () => {
    let release = () => {};
    const gate = new Promise<void>((resolve) => (release = resolve));
    reload.mockImplementationOnce(async () => {
      await gate;
      return loaded;
    });
    await catchUp();
    await board.make(createTask("t1"));
    serverCards = [
      { id: "c1", name: "Todo", description: "", tasks: [task("t1")] },
    ];
    release();

    await vi.waitFor(() => expect(reload).toHaveBeenCalledTimes(2));
    expect(taskIds(board.cards())).toEqual(["t1"]);
  });

  it("leaves the board as it is when loading it fails", async () => {
    reload.mockRejectedValueOnce(new Error("offline"));
    await board.make(createTask("t1"));
    await catchUp();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(taskIds(board.cards())).toEqual(["t1"]);
  });

  it("ignores other boards' sockets", async () => {
    socket.open("b2");
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(reload).not.toHaveBeenCalled();
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
    socket.receive(createTask("t1"));
    stop();

    await vi.waitFor(async () =>
      expect(taskIds((await loadBoard(me.id, "b1"))!.cards)).toEqual(["t1"]),
    );
  });
});

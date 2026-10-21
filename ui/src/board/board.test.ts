import "fake-indexeddb/auto";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BoardChange } from "@/offline/changes";

const fake = await vi.hoisted(async () =>
  (await import("@/utils/socket.fake")).fakeSocket(),
);
vi.mock("@/utils/socket", () => ({ socket: fake.socket }));

const network = { onLine: true };
vi.stubGlobal("navigator", network);
vi.stubGlobal("window", new EventTarget());

const { deleteCopy, loadBoard, loadPending, saveBoard } =
  await import("@/offline/copy");
const { setOnRefused, startSync, stopSync, sync } =
  await import("@/offline/sync");
const { openBoard } = await import("./board");

const me = { id: "u1", email: "me@example.com", name: "Me" };

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

let board: ReturnType<typeof openBoard>;
let stop: () => void;
const refused = vi.fn();

const sentIds = () =>
  fake.sent.map((change) => ("taskId" in change ? change.taskId : null));

beforeEach(async () => {
  fake.reset();
  fake.open.add("b1");
  network.onLine = true;
  refused.mockClear();
  setOnRefused(refused);
  await startSync(me);
  board = openBoard(me.id, "b1", loaded);
  stop = board.start();
});

afterEach(async () => {
  stop();
  stopSync();
  await deleteCopy(me.id);
});

describe("make", () => {
  it("shows the change at once, then sends it over the board's socket", async () => {
    const made = board.make(createTask("t1"));
    expect(taskIds(board.cards())).toEqual(["t1"]);

    await made;
    expect(fake.sent).toEqual([createTask("t1")]);
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("keeps the board's socket open while the board is", () => {
    expect(fake.joined).toEqual(["b1"]);
    stop();
    expect(fake.joined).toEqual([]);
    stop = board.start();
  });

  it("keeps the change as pending offline, and sends it on reconnect", async () => {
    network.onLine = false;
    await board.make(createTask("t1"));

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(fake.sent).toEqual([]);
    expect(await loadPending(me.id)).toHaveLength(1);

    network.onLine = true;
    await sync();
    expect(await loadPending(me.id)).toEqual([]);
    expect(fake.sent).toEqual([createTask("t1")]);
  });

  it.each([
    ["can't be reached", "unreachable" as const],
    ["answers with a server error", 503],
  ])("keeps the change as pending when the server %s", async (_, status) => {
    fake.status = () => status;
    await board.make(createTask("t1"));

    expect(taskIds(board.cards())).toEqual(["t1"]);
    expect(await loadPending(me.id)).toHaveLength(1);
    expect(refused).not.toHaveBeenCalled();

    // Later changes wait behind it, so they still arrive in order.
    fake.sent.length = 0;
    fake.status = () => 200;
    await board.make(createTask("t2"));
    await sync();
    expect(sentIds()).toEqual(["t1", "t2"]);
  });

  it("reloads the board and says so when the server refuses", async () => {
    fake.status = () => 404;
    await board.make(createTask("t1"));

    expect(refused).toHaveBeenCalledOnce();
    expect(await loadPending(me.id)).toEqual([]);
  });

  it("sends changes in the order they were made", async () => {
    await Promise.all(
      ["t1", "t2", "t3"].map((id) => board.make(createTask(id))),
    );
    expect(sentIds()).toEqual(["t1", "t2", "t3"]);
  });
});

describe("teammates' changes", () => {
  it("show on the board they were made on", () => {
    const listener = vi.fn();
    board.subscribe(listener);

    fake.receive(createTask("t1"));
    fake.receive({ ...createTask("t2"), boardId: "b2" });

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
    fake.receive(createTask("t1"));
    stop();

    await vi.waitFor(async () =>
      expect(taskIds((await loadBoard(me.id, "b1"))!.cards)).toEqual(["t1"]),
    );
  });
});

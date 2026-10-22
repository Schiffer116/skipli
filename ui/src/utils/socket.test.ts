import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BoardChange } from "@/offline/changes";

// What the server says about the board when a socket drops.
let boardStatus = 200;
// Who the server says has the board open.
let presenceBody: unknown[] = [];
vi.mock("@/utils/apiFetch", () => ({
  apiFetch: async (url: string) =>
    new Response(
      url.endsWith("/presence") ? JSON.stringify(presenceBody) : "",
      {
        status: boardStatus,
      },
    ),
}));

// The changes this app made, as its Pending changes know them.
vi.mock("@/offline/pending", () => ({
  isOwnChange: (id: string) => id === "mine",
}));

// A browser WebSocket the test plays the server for.
class FakeWebSocket {
  static OPEN = 1;
  static all: FakeWebSocket[] = [];
  readyState = 0;
  sent: unknown[] = [];
  onopen?: () => void;
  onmessage?: (e: { data: string }) => void;
  onclose?: () => void;

  url: string;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.all.push(this);
  }
  send(data: string) {
    this.sent.push(JSON.parse(data));
  }
  close() {
    if (this.readyState === 3) return;
    this.readyState = 3;
    this.onclose?.();
  }
  // Server side:
  accept() {
    this.readyState = 1;
    this.onopen?.();
  }
  reply(message: unknown) {
    this.onmessage?.({ data: JSON.stringify(message) });
  }
}

vi.stubGlobal("WebSocket", FakeWebSocket);
vi.stubGlobal("location", { protocol: "https:", host: "skipli.test" });
vi.stubGlobal("navigator", { onLine: true });
vi.stubGlobal("window", new EventTarget());

const createTask: BoardChange = {
  type: "createTask",
  boardId: "b1",
  cardId: "c1",
  taskId: "t1",
  name: "Ship",
};

let socket: typeof import("./socket").socket;
const leaves: (() => void)[] = [];
const join = (boardId: string) => {
  const leave = socket.join(boardId);
  leaves.push(leave);
  return leave;
};
const last = () => FakeWebSocket.all.at(-1)!;
const flush = () => vi.advanceTimersByTimeAsync(0);

beforeEach(async () => {
  vi.useFakeTimers();
  vi.resetModules();
  FakeWebSocket.all = [];
  boardStatus = 200;
  presenceBody = [];
  ({ socket } = await import("./socket"));
});

afterEach(() => {
  leaves.splice(0).forEach((leave) => leave());
  vi.useRealTimers();
});

describe("a board's socket", () => {
  it("opens on its board", () => {
    join("b1");
    expect(last().url).toBe("wss://skipli.test/api/ws?boardId=b1");
  });

  it("passes on teammates' changes", () => {
    const received = vi.fn();
    socket.onChange(received);
    join("b1");
    last().accept();
    last().reply({ type: "change", id: "theirs", change: createTask });
    expect(received).toHaveBeenCalledWith(createTask);
  });

  it("ignores the broadcast of a change this app made", () => {
    const received = vi.fn();
    const updated = vi.fn();
    socket.onChange(received);
    socket.onUpdated(updated);
    join("b1");
    last().accept();

    const id = "mine";
    last().reply({ type: "change", id, change: createTask });
    last().reply({ type: "board", id, board: { name: "A", description: "" } });
    expect(received).not.toHaveBeenCalled();
    expect(updated).not.toHaveBeenCalled();
  });

  it("passes on renames and deletions of the board", () => {
    const updated = vi.fn();
    const deleted = vi.fn();
    socket.onUpdated(updated);
    socket.onDeleted(deleted);
    join("b1");
    last().accept();

    last().reply({
      type: "board",
      board: { name: "Plans", description: "Q4" },
    });
    expect(updated).toHaveBeenCalledWith("b1", {
      name: "Plans",
      description: "Q4",
    });
    last().reply({ type: "deleted" });
    expect(deleted).toHaveBeenCalledWith("b1");
  });

  it("pings every few minutes so API Gateway doesn't close it as idle", async () => {
    join("b1");
    last().accept();
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(last().sent).toEqual([{ action: "ping" }]);
  });

  it("reconnects after it drops, and says each time it opens", async () => {
    const opened = vi.fn();
    socket.onOpen(opened);
    join("b1");
    last().accept();
    last().close();

    await vi.advanceTimersByTimeAsync(3_000);
    expect(FakeWebSocket.all).toHaveLength(2);
    last().accept();
    expect(opened.mock.calls).toEqual([["b1"], ["b1"]]);
  });

  it("says the board was deleted when it's gone on reconnecting, and stops", async () => {
    const deleted = vi.fn();
    socket.onDeleted(deleted);
    join("b1");
    last().accept();
    boardStatus = 404;
    last().close();
    await flush();

    expect(deleted).toHaveBeenCalledWith("b1");
    await vi.advanceTimersByTimeAsync(60_000);
    expect(FakeWebSocket.all).toHaveLength(1);
  });

  it("stops reconnecting once the person is no longer a member", async () => {
    join("b1");
    last().accept();
    boardStatus = 403;
    last().close();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(FakeWebSocket.all).toHaveLength(1);
  });

  it("is shared, and closes once nobody needs it", () => {
    const leaveA = join("b1");
    const leaveB = join("b1");
    expect(FakeWebSocket.all).toHaveLength(1);
    leaveA();
    expect(last().readyState).not.toBe(3);
    leaveB();
    expect(last().readyState).toBe(3);
  });
});

describe("a socket opened ahead of its board", () => {
  it("keeps what it brings until the board takes it", () => {
    leaves.push(socket.joinEarly("b1"));
    last().accept();
    last().reply({ type: "change", id: "theirs", change: createTask });

    expect(socket.takeEarly("b1")).toEqual([createTask]);
    expect(socket.takeEarly("b1")).toBeNull();
  });

  it("says when it opened, while it's open", () => {
    const now = vi.spyOn(performance, "now").mockReturnValue(42);
    leaves.push(socket.joinEarly("b1"));
    expect(socket.openedAt("b1")).toBeUndefined();
    last().accept();
    expect(socket.openedAt("b1")).toBe(42);
    last().close();
    expect(socket.openedAt("b1")).toBeUndefined();
    now.mockRestore();
  });

  it("stays open once its board joins", async () => {
    socket.joinEarly("b1");
    join("b1");
    last().accept();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(last().readyState).toBe(FakeWebSocket.OPEN);
  });

  it("closes after a while when no board joins", async () => {
    socket.joinEarly("b1");
    last().accept();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(last().readyState).toBe(3);
    expect(socket.takeEarly("b1")).toBeNull();
  });

  it("closes at once when the board doesn't load", () => {
    const leave = socket.joinEarly("b1");
    last().accept();
    leave();
    expect(last().readyState).toBe(3);
  });
});

describe("Presence", () => {
  const ann = { id: "u1", name: "Ann", email: "ann@example.com" };
  const bo = { id: "u2", name: "Bo", email: "bo@example.com" };

  it("reads who is here when the socket opens", async () => {
    const heard = vi.fn();
    socket.onPresence(heard);
    presenceBody = [ann];
    join("b1");
    last().accept();
    await flush();
    expect(heard).toHaveBeenCalledExactlyOnceWith("b1", [ann]);
  });

  it("follows the people coming and going", () => {
    const heard = vi.fn();
    socket.onPresence(heard);
    join("b1");
    last().accept();
    last().reply({ type: "presence", people: [ann, bo] });
    last().reply({ type: "presence" });
    expect(heard).toHaveBeenNthCalledWith(1, "b1", [ann, bo]);
    expect(heard).toHaveBeenNthCalledWith(2, "b1", []);
  });

  it("drops a list read that a push overtook", async () => {
    const heard = vi.fn();
    socket.onPresence(heard);
    presenceBody = [ann];
    join("b1");
    last().accept();
    last().reply({ type: "presence", people: [ann, bo] });
    await flush();
    expect(heard).toHaveBeenCalledExactlyOnceWith("b1", [ann, bo]);
  });

  it("keeps the latest list for a listener that comes late", async () => {
    presenceBody = [ann];
    join("b1");
    last().accept();
    await flush();
    expect(socket.presentAt("b1")).toEqual([ann]);
    last().close();
    expect(socket.presentAt("b1")).toEqual([]);
  });

  it("shows nobody while the socket is down", () => {
    const heard = vi.fn();
    join("b1");
    last().accept();
    socket.onPresence(heard);
    last().close();
    expect(heard).toHaveBeenCalledExactlyOnceWith("b1", []);
  });
});

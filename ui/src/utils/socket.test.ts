import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BoardChange } from "@/offline/changes";

// What the server says about the board when a socket drops.
let boardStatus = 200;
vi.mock("@/utils/apiFetch", () => ({
  apiFetch: async () => new Response("", { status: boardStatus }),
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
  ({ socket } = await import("./socket"));
});

afterEach(() => {
  leaves.splice(0).forEach((leave) => leave());
  vi.useRealTimers();
});

describe("send", () => {
  it("sends a change on its board's socket and resolves with the ack", async () => {
    join("b1");
    last().accept();
    expect(last().url).toBe("wss://skipli.test/api/ws?boardId=b1");

    const answer = socket.send(createTask);
    const [message] = last().sent as { id: string }[];
    expect(message).toEqual({
      action: "change",
      id: message.id,
      change: createTask,
    });

    last().reply({ type: "ack", id: "other", status: 200 });
    last().reply({
      type: "ack",
      id: message.id,
      status: 404,
      error: "card not found",
    });
    expect(await answer).toEqual({ status: 404, text: "card not found" });
  });

  it("has no answer while the socket is still opening", async () => {
    join("b1");
    expect(await socket.send(createTask)).toBeNull();
    expect(last().sent).toEqual([]);
  });

  it("opens the socket for a change sent with wait, and closes it after", async () => {
    const answer = socket.send(createTask, { wait: true });
    await flush();
    last().accept();
    await flush();
    const [message] = last().sent as { id: string }[];
    last().reply({ type: "ack", id: message.id, status: 200 });

    expect(await answer).toEqual({ status: 200, text: "" });
    expect(last().readyState).toBe(3);
  });

  it("gives up on a socket that doesn't open in time", async () => {
    const answer = socket.send(createTask, { wait: true });
    await vi.advanceTimersByTimeAsync(10_000);
    expect(await answer).toBeNull();
  });

  it("answers a change to a board that's gone with the board's status", async () => {
    boardStatus = 404;
    const answer = socket.send(createTask, { wait: true });
    await flush();
    last().close();
    expect(await answer).toEqual({ status: 404, text: "" });
    // And doesn't keep trying to open it.
    await vi.advanceTimersByTimeAsync(60_000);
    expect(FakeWebSocket.all).toHaveLength(1);
  });

  it("has no answer when the socket closes first, or the ack never comes", async () => {
    join("b1");
    last().accept();
    const closed = socket.send(createTask);
    last().close();
    expect(await closed).toBeNull();

    await vi.advanceTimersByTimeAsync(3_000);
    last().accept();
    const unanswered = socket.send(createTask);
    await vi.advanceTimersByTimeAsync(15_000);
    expect(await unanswered).toBeNull();
  });
});

describe("a board's socket", () => {
  it("passes on teammates' changes", () => {
    const received = vi.fn();
    socket.onChange(received);
    join("b1");
    last().accept();
    last().reply({ type: "change", change: createTask });
    expect(received).toHaveBeenCalledWith(createTask);
  });

  it("pings every few minutes so API Gateway doesn't close it as idle", async () => {
    join("b1");
    last().accept();
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    expect(last().sent).toEqual([{ action: "ping" }]);
  });

  it("reconnects after it drops, and says when it's open again", async () => {
    const opened = vi.fn();
    socket.onOpen(opened);
    join("b1");
    last().accept();
    last().close();

    await vi.advanceTimersByTimeAsync(3_000);
    expect(FakeWebSocket.all).toHaveLength(2);
    last().accept();
    expect(opened).toHaveBeenCalledTimes(2);
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

import type { BoardChange } from "@/offline/changes";

import type { Answer, socket } from "./socket";

// A stand-in for the board sockets, for tests of what sends over them:
// records the changes sent, answers each with status(), and plays back
// teammates' changes. Use with
// vi.mock("@/utils/socket", () => ({ socket: fake.socket })).
export function fakeSocket() {
  const changeHandlers = new Set<(change: BoardChange) => void>();
  const openHandlers = new Set<() => void>();

  const state: {
    sent: BoardChange[];
    // The board sockets that are open; sending on another one waits for
    // it only when asked to, and then it opens.
    open: Set<string>;
    joined: string[];
    status: () => number | "unreachable";
  } = { sent: [], open: new Set(), joined: [], status: () => 200 };

  const socketImpl: typeof socket = {
    join(boardId: string) {
      state.joined.push(boardId);
      return () => {
        state.joined.splice(state.joined.indexOf(boardId), 1);
      };
    },
    async send(change: BoardChange, { wait = false } = {}) {
      if (!state.open.has(change.boardId)) {
        if (!wait) return null;
        state.open.add(change.boardId);
      }
      const status = state.status();
      if (status === "unreachable") return null;
      state.sent.push(change);
      return { status, text: `status ${status}` } satisfies Answer;
    },
    onChange(handler: (change: BoardChange) => void) {
      changeHandlers.add(handler);
      return () => void changeHandlers.delete(handler);
    },
    onOpen(handler: () => void) {
      openHandlers.add(handler);
      return () => void openHandlers.delete(handler);
    },
  };

  return Object.assign(state, {
    socket: socketImpl,

    receive: (change: BoardChange) =>
      changeHandlers.forEach((handler) => handler(change)),

    // Opens a board's socket, as when it connects after the page loaded.
    connect(boardId: string) {
      state.open.add(boardId);
      openHandlers.forEach((handler) => handler());
    },

    reset() {
      state.sent.length = 0;
      state.open.clear();
      state.status = () => 200;
    },
  });
}

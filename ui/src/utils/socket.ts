import type { BoardChange } from "@/offline/changes";

import { apiFetch } from "./apiFetch";

// The server's answer to a change: a status as HTTP would give it, and the
// reason when it refused.
export type Answer = { status: number; text: string };

// API Gateway closes a connection that's been quiet for 10 minutes.
const KEEPALIVE = 5 * 60_000;
// A change with no answer by then is treated as not having reached the
// server; sending it again is safe (a create answers 409, an update sets
// the same values).
const ANSWER_TIMEOUT = 15_000;
// How long sending a Pending change waits for its board's socket to open.
const OPEN_TIMEOUT = 10_000;
const RETRY = 3_000;

// One board's socket, shared by everyone that needs it open.
type Room = {
  boardId: string;
  ws: WebSocket | null;
  holders: number;
  // Waiting for an answer, by message id.
  answers: Map<string, (answer: Answer | null) => void>;
  // Called when the socket opens or fails to.
  opening: Set<(opened: boolean) => void>;
  // Set once the board can't be opened at all (gone, or no longer ours).
  closedFor: Answer | null;
  retry?: ReturnType<typeof setTimeout>;
  keepalive?: ReturnType<typeof setInterval>;
  waitForOnline?: () => void;
};

const rooms = new Map<string, Room>();
const changeHandlers = new Set<(change: BoardChange) => void>();
const openHandlers = new Set<() => void>();
let lastId = 0;

const isOpen = (room?: Room) => room?.ws?.readyState === WebSocket.OPEN;

function open(room: Room) {
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  const ws = new WebSocket(
    `${protocol}://${location.host}/api/ws?boardId=${encodeURIComponent(room.boardId)}`,
  );
  room.ws = ws;

  ws.onopen = () => {
    room.keepalive = setInterval(
      () => ws.send(JSON.stringify({ action: "ping" })),
      KEEPALIVE,
    );
    room.opening.forEach((done) => done(true));
    openHandlers.forEach((handler) => handler());
  };

  ws.onmessage = (e) => {
    const message = JSON.parse(e.data);
    if (message.type === "ack") {
      room.answers.get(message.id)?.({
        status: message.status,
        text: message.error ?? "",
      });
    } else if (message.type === "change") {
      changeHandlers.forEach((handler) => handler(message.change));
    }
  };

  ws.onclose = () => {
    clearInterval(room.keepalive);
    room.answers.forEach((answer) => answer(null));
    if (room.ws !== ws) return;
    room.ws = null;
    void reconnect(room);
  };
}

// Opens a closed socket again, unless the board itself is the problem.
async function reconnect(room: Room) {
  // Asking for the board refreshes the session if it expired while
  // disconnected, and says whether the board can still be opened.
  let refusal: Answer | null = null;
  try {
    const res = await apiFetch(
      `/api/boards/${encodeURIComponent(room.boardId)}`,
    );
    if (res.status === 403 || res.status === 404) {
      refusal = { status: res.status, text: await res.text() };
    }
  } catch {
    // Still unreachable: try again below.
  }
  if (rooms.get(room.boardId) !== room) return;
  if (refusal) {
    room.closedFor = refusal;
    room.opening.forEach((done) => done(false));
    return;
  }
  room.opening.forEach((done) => done(false));

  // Offline there's nothing to reach, so wait for the browser to say the
  // connection is back instead of retrying every few seconds.
  const again = () => {
    if (rooms.get(room.boardId) === room) open(room);
  };
  if (navigator.onLine) {
    room.retry = setTimeout(again, RETRY);
  } else {
    room.waitForOnline = () => {
      room.waitForOnline = undefined;
      again();
    };
    window.addEventListener("online", room.waitForOnline, { once: true });
  }
}

function join(boardId: string) {
  let room = rooms.get(boardId);
  if (!room) {
    room = {
      boardId,
      ws: null,
      holders: 0,
      answers: new Map(),
      opening: new Set(),
      closedFor: null,
    };
    rooms.set(boardId, room);
    open(room);
  }
  room.holders++;

  const joined = room;
  let left = false;
  return () => {
    if (left) return;
    left = true;
    if (--joined.holders > 0) return;
    rooms.delete(boardId);
    clearTimeout(joined.retry);
    clearInterval(joined.keepalive);
    if (joined.waitForOnline) {
      window.removeEventListener("online", joined.waitForOnline);
    }
    const ws = joined.ws;
    joined.ws = null;
    ws?.close();
    joined.answers.forEach((answer) => answer(null));
    joined.opening.forEach((done) => done(false));
  };
}

// Resolves once the room's socket is open: true, or false if it failed to
// open in time.
function opened(room: Room) {
  if (isOpen(room)) return Promise.resolve(true);
  return new Promise<boolean>((resolve) => {
    const done = (ok: boolean) => {
      clearTimeout(timer);
      room.opening.delete(done);
      resolve(ok);
    };
    const timer = setTimeout(() => done(false), OPEN_TIMEOUT);
    room.opening.add(done);
  });
}

export const socket = {
  // Keeps a board's socket open, reconnecting as needed, until the
  // returned function is called.
  join,

  // Sends a Board change over its board's socket. Resolves with the
  // server's answer, or null when there was none: the socket isn't open
  // (with wait, the socket is opened for it and given a few seconds), it
  // closed first, or the answer took too long.
  async send(
    change: BoardChange,
    { wait = false }: { wait?: boolean } = {},
  ): Promise<Answer | null> {
    let room = rooms.get(change.boardId);
    if (room?.closedFor) return room.closedFor;
    let leave: (() => void) | undefined;
    try {
      if (!isOpen(room)) {
        if (!wait) return null;
        leave = join(change.boardId);
        room = rooms.get(change.boardId)!;
        if (!(await opened(room))) return room.closedFor;
      }
      return await answer(room!, change);
    } finally {
      leave?.();
    }
  },

  // Calls back with each change a teammate made to a board whose socket
  // is open.
  onChange(handler: (change: BoardChange) => void) {
    changeHandlers.add(handler);
    return () => void changeHandlers.delete(handler);
  },

  // Calls back whenever a board's socket opens, so waiting changes can go.
  onOpen(handler: () => void) {
    openHandlers.add(handler);
    return () => void openHandlers.delete(handler);
  },
};

function answer(room: Room, change: BoardChange) {
  const id = String(++lastId);
  return new Promise<Answer | null>((resolve) => {
    const done = (answer: Answer | null) => {
      clearTimeout(timer);
      room.answers.delete(id);
      resolve(answer);
    };
    const timer = setTimeout(() => done(null), ANSWER_TIMEOUT);
    room.answers.set(id, done);
    room.ws!.send(JSON.stringify({ action: "change", id, change }));
  });
}

import type { BoardChange } from "@/offline/changes";
import { newId } from "@/offline/ids";

import { apiFetch } from "./apiFetch";

// What the server says about a board's name and description when a
// teammate changes them.
export type BoardDetail = { name: string; description: string };

// API Gateway closes a connection that's been quiet for 10 minutes.
const KEEPALIVE = 5 * 60_000;
const RETRY = 3_000;
// How long a socket opened ahead of its board (joinEarly) stays open for the
// board to take it over.
const EARLY_HOLD = 10_000;

// The ids of the writes this app made. The server broadcasts every write it
// accepts to everyone on the board, the sender included, and what the app
// already shows mustn't be applied again.
const OWN_IDS_KEPT = 500;
const ownIds = new Set<string>();

// An id for a write the app is about to send, to put in its X-Change-Id
// header so the server's broadcast of it can be recognised as ours.
export function newChangeId() {
  const id = newId();
  ownIds.add(id);
  if (ownIds.size > OWN_IDS_KEPT) {
    ownIds.delete(ownIds.values().next().value!);
  }
  return id;
}

// One board's socket, shared by everyone that needs it open.
type Room = {
  boardId: string;
  ws: WebSocket | null;
  holders: number;
  // When the socket last opened (performance.now()), while it's open.
  openedAt?: number;
  // Teammates' changes received while opened ahead of its board, for the
  // board to take once it opens.
  early: BoardChange[] | null;
  retry?: ReturnType<typeof setTimeout>;
  keepalive?: ReturnType<typeof setInterval>;
  waitForOnline?: () => void;
};

const rooms = new Map<string, Room>();
const changeHandlers = new Set<(change: BoardChange) => void>();
const updatedHandlers = new Set<
  (boardId: string, detail: BoardDetail) => void
>();
const deletedHandlers = new Set<(boardId: string) => void>();
const openHandlers = new Set<(boardId: string) => void>();

function open(room: Room) {
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  const ws = new WebSocket(
    `${protocol}://${location.host}/api/ws?boardId=${encodeURIComponent(room.boardId)}`,
  );
  room.ws = ws;

  ws.onopen = () => {
    room.openedAt = performance.now();
    room.keepalive = setInterval(
      () => ws.send(JSON.stringify({ action: "ping" })),
      KEEPALIVE,
    );
    openHandlers.forEach((handler) => handler(room.boardId));
  };

  ws.onmessage = (e) => {
    const message = JSON.parse(e.data);
    if (message.id && ownIds.has(message.id)) return;
    if (message.type === "change") {
      room.early?.push(message.change);
      changeHandlers.forEach((handler) => handler(message.change));
    } else if (message.type === "board") {
      updatedHandlers.forEach((handler) =>
        handler(room.boardId, message.board),
      );
    } else if (message.type === "deleted") {
      deletedHandlers.forEach((handler) => handler(room.boardId));
    }
  };

  ws.onclose = () => {
    clearInterval(room.keepalive);
    if (room.ws !== ws) return;
    room.ws = null;
    room.openedAt = undefined;
    void reconnect(room);
  };
}

// Opens a closed socket again, unless the board itself is the problem.
async function reconnect(room: Room) {
  // Asking for the board refreshes the session if it expired while
  // disconnected, and says whether the board can still be opened.
  let status = 0;
  try {
    status = (await apiFetch(`/api/boards/${encodeURIComponent(room.boardId)}`))
      .status;
  } catch {
    // Still unreachable: try again below.
  }
  if (rooms.get(room.boardId) !== room) return;
  if (status === 404) {
    // Deleted while this socket was down, so nobody could say so.
    deletedHandlers.forEach((handler) => handler(room.boardId));
    return;
  }
  if (status === 403) return;

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
    room = { boardId, ws: null, holders: 0, early: null };
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
  };
}

// Opens a board's socket ahead of the board, as its page starts loading, and
// keeps the changes it brings until the board takes them (takeEarly). It
// closes after a while unless the board has joined it by then, or at once
// when the returned function is called.
function joinEarly(boardId: string) {
  const leave = join(boardId);
  const room = rooms.get(boardId)!;
  room.early ??= [];
  const timer = setTimeout(done, EARLY_HOLD);
  function done() {
    clearTimeout(timer);
    room.early = null;
    leave();
  }
  return done;
}

const on =
  <A extends unknown[]>(handlers: Set<(...args: A) => void>) =>
  (handler: (...args: A) => void) => {
    handlers.add(handler);
    return () => void handlers.delete(handler);
  };

// A board's socket carries news out only: what teammates saved. Changes are
// made over HTTP.
export const socket = {
  // Keeps a board's socket open, reconnecting as needed, until the
  // returned function is called.
  join,

  joinEarly,

  // The changes received since the socket was opened early, once; null
  // when it wasn't, or they were already taken.
  takeEarly(boardId: string): BoardChange[] | null {
    const room = rooms.get(boardId);
    const early = room?.early ?? null;
    if (room) room.early = null;
    return early;
  },

  // When the board's socket opened (performance.now()), if it's open: it
  // has carried every change saved since.
  openedAt: (boardId: string) => rooms.get(boardId)?.openedAt,

  // Calls back with each change a teammate made to a board whose socket
  // is open.
  onChange: on(changeHandlers),

  // Calls back when a teammate renames the board or changes its
  // description.
  onUpdated: on(updatedHandlers),

  // Calls back when the board was deleted.
  onDeleted: on(deletedHandlers),

  // Calls back whenever a board's socket opens, the first time or after a
  // drop. Whatever was saved while it wasn't open was never sent, so this
  // is when to load the board again.
  onOpen: on(openHandlers),
};

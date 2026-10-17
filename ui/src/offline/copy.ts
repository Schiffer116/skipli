import { deleteDB, openDB, type DBSchema, type IDBPDatabase } from "idb";

import type { CardType } from "@/pages/Boards/Board/Card";
import type { Me, Member } from "@/pages/Boards/loaders";

import { applyToList, type BoardList, type Change } from "./changes";

// The Offline copy: what one person's boards looked like when last seen
// on this device, plus their Pending and Rejected changes. Each person
// gets a database of their own, so someone else signing in here never sees
// it, and signing out deletes just that one.

export type SavedBoard = {
  id: string;
  name: string;
  description: string;
  owner: string;
  members: Member[];
  cards: CardType[];
  savedAt: number;
};

export type SavedList = BoardList & { savedAt: number };

export type PendingChange = { seq: number; change: Change; at: number };

export type RejectedChange = {
  seq: number;
  change: Change;
  status: number;
  message: string;
  at: number;
};

interface Schema extends DBSchema {
  meta: {
    key: "me" | "list" | "rejectedSeen";
    value: Me | SavedList | number;
  };
  boards: { key: string; value: SavedBoard };
  pending: { key: number; value: PendingChange };
  rejected: { key: number; value: RejectedChange };
}

const databases = new Map<string, Promise<IDBPDatabase<Schema>>>();

const nameFor = (userId: string) => `skipli-offline-${userId}`;

function open(userId: string) {
  let db = databases.get(userId);
  if (!db) {
    db = openDB<Schema>(nameFor(userId), 1, {
      upgrade(db) {
        db.createObjectStore("meta");
        db.createObjectStore("boards", { keyPath: "id" });
        db.createObjectStore("pending", {
          keyPath: "seq",
          autoIncrement: true,
        });
        db.createObjectStore("rejected", {
          keyPath: "seq",
          autoIncrement: true,
        });
      },
    });
    databases.set(userId, db);
  }
  return db;
}

export async function deleteCopy(userId: string) {
  const db = databases.get(userId);
  databases.delete(userId);
  (await db)?.close();
  await deleteDB(nameFor(userId));
}

export async function loadMe(userId: string) {
  return (await (await open(userId)).get("meta", "me")) as Me | undefined;
}

export async function saveMe(me: Me) {
  await (await open(me.id)).put("meta", me, "me");
}

export async function loadList(userId: string) {
  return (await (await open(userId)).get("meta", "list")) as
    SavedList | undefined;
}

export async function saveList(userId: string, list: BoardList) {
  const db = await open(userId);
  const tx = db.transaction(["meta", "boards"], "readwrite");
  await tx.objectStore("meta").put({ ...list, savedAt: Date.now() }, "list");

  // A board missing from the list was deleted, or this person lost access
  // to it: it leaves the offline copy too.
  const listed = new Set(
    [...list.boards, ...list.teamBoards].map((board) => board.id),
  );
  for (const id of await tx.objectStore("boards").getAllKeys()) {
    if (!listed.has(id)) await tx.objectStore("boards").delete(id);
  }
  await tx.done;
}

export async function loadBoard(userId: string, boardId: string) {
  return (await open(userId)).get("boards", boardId);
}

export async function loadBoards(userId: string) {
  return (await open(userId)).getAll("boards");
}

export async function saveBoard(
  userId: string,
  board: Omit<SavedBoard, "savedAt">,
) {
  await (await open(userId)).put("boards", { ...board, savedAt: Date.now() });
}

// Updates part of a saved board, if this device has it.
export async function updateBoard(
  userId: string,
  boardId: string,
  update: Partial<Omit<SavedBoard, "id" | "savedAt">>,
) {
  const db = await open(userId);
  const tx = db.transaction("boards", "readwrite");
  const board = await tx.store.get(boardId);
  if (board) await tx.store.put({ ...board, ...update, savedAt: Date.now() });
  await tx.done;
}

export async function forgetBoard(userId: string, boardId: string) {
  await (await open(userId)).delete("boards", boardId);
}

// Applies a change the server accepted, or one waiting to be sent, to the
// saved board list, and to the saved board it creates, renames or deletes.
export async function applyChange(me: Me, change: Change) {
  const db = await open(me.id);
  const tx = db.transaction(["meta", "boards"], "readwrite");
  const meta = tx.objectStore("meta");
  const boards = tx.objectStore("boards");

  const list = (await meta.get("list")) as SavedList | undefined;
  if (list) {
    await meta.put({ ...list, ...applyToList(list, change, me.id) }, "list");
  }

  if (change.type === "createBoard") {
    await boards.put({
      id: change.boardId,
      name: change.name,
      description: change.description,
      owner: me.id,
      members: [me],
      cards: [],
      savedAt: Date.now(),
    });
  } else if (change.type === "updateBoard") {
    const board = await boards.get(change.boardId);
    if (board) {
      await boards.put({
        ...board,
        name: change.name,
        description: change.description,
      });
    }
  } else if (change.type === "deleteBoard") {
    await boards.delete(change.boardId);
  }
  await tx.done;
}

export async function addPending(userId: string, change: Change) {
  await (
    await open(userId)
  ).add("pending", {
    change,
    at: Date.now(),
  } as PendingChange);
}

export async function loadPending(userId: string) {
  return (await open(userId)).getAll("pending");
}

// The oldest Pending change, the next one to send.
export async function nextPending(userId: string) {
  const [pending] = await (await open(userId)).getAll("pending", null, 1);
  return pending as PendingChange | undefined;
}

export async function removePending(userId: string, seq: number) {
  await (await open(userId)).delete("pending", seq);
}

export async function reject(
  userId: string,
  pending: PendingChange,
  status: number,
  message: string,
) {
  const db = await open(userId);
  const tx = db.transaction(["pending", "rejected"], "readwrite");
  await tx.objectStore("rejected").add({
    change: pending.change,
    status,
    message,
    at: Date.now(),
  } as RejectedChange);
  await tx.objectStore("pending").delete(pending.seq);
  await tx.done;
}

export async function loadRejected(userId: string) {
  return (await open(userId)).getAll("rejected");
}

// How many Rejected changes this person has already been told about.
export async function loadRejectedSeen(userId: string) {
  return ((await (await open(userId)).get("meta", "rejectedSeen")) ??
    0) as number;
}

export async function saveRejectedSeen(userId: string, seen: number) {
  await (await open(userId)).put("meta", seen, "rejectedSeen");
}

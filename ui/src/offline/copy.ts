import { deleteDB, openDB, type DBSchema, type IDBPDatabase } from "idb";

import { apply } from "@/board/apply";
import type { CardType } from "@/pages/Boards/Board/Card";
import type { Me, Member } from "@/pages/Boards/loaders";

import {
  applyToList,
  isBoardChange,
  type BoardList,
  type Change,
} from "./changes";
import { newId } from "./ids";
import type { PendingStore } from "./pending";

// The Offline copy: what one person's boards looked like when last seen
// on this device, plus their Pending changes. Each person
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
  // A change to it was dropped, so it may show what the server never
  // had: online, it loads from the server next time it's opened.
  outOfDate?: true;
};

export type SavedList = BoardList & { savedAt: number; outOfDate?: true };

// id is the change id every attempt to send it carries (X-Change-Id).
export type PendingChange = {
  seq: number;
  id: string;
  change: Change;
  at: number;
};

interface Schema extends DBSchema {
  meta: { key: "me" | "list"; value: Me | SavedList };
  boards: { key: string; value: SavedBoard };
  pending: { key: number; value: PendingChange };
}

const databases = new Map<string, Promise<IDBPDatabase<Schema>>>();

const nameFor = (userId: string) => `skipli-offline-${userId}`;

function open(userId: string) {
  let db = databases.get(userId);
  if (!db) {
    db = openDB<Schema>(nameFor(userId), 2, {
      async upgrade(db, oldVersion, _, tx) {
        if (oldVersion < 1) {
          db.createObjectStore("meta");
          db.createObjectStore("boards", { keyPath: "id" });
          db.createObjectStore("pending", {
            keyPath: "seq",
            autoIncrement: true,
          });
          return;
        }
        // Version 1 kept refused changes for a review that never came, and
        // gave pending ones no change id.
        if (oldVersion < 2) {
          db.deleteObjectStore("rejected" as never);
          await tx.objectStore("meta").delete("rejectedSeen" as never);
          let cursor = await tx.objectStore("pending").openCursor();
          while (cursor) {
            await cursor.update({ ...cursor.value, id: newId() });
            cursor = await cursor.continue();
          }
        }
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

// A board with a change laid on top: a Board change to its cards, or a
// rename of it.
export function withChange<B extends Omit<SavedBoard, "savedAt">>(
  board: B,
  change: Change,
): B {
  if (!("boardId" in change) || change.boardId !== board.id) return board;
  if (isBoardChange(change)) {
    return { ...board, cards: apply(board.cards, change) };
  }
  if (change.type === "updateBoard") {
    return { ...board, name: change.name, description: change.description };
  }
  return board;
}

export async function loadPending(userId: string) {
  return (await open(userId)).getAll("pending");
}

// Where one person's Pending changes are kept.
export function pendingStore(me: Me): PendingStore {
  return {
    // Keeps a change and shows it in the Offline copy, both or neither.
    async queue(change, id) {
      const db = await open(me.id);
      const tx = db.transaction(["pending", "meta", "boards"], "readwrite");
      const meta = tx.objectStore("meta");
      const boards = tx.objectStore("boards");
      await tx.objectStore("pending").add({
        id,
        change,
        at: Date.now(),
      } as PendingChange);

      const list = (await meta.get("list")) as SavedList | undefined;
      if (list) {
        await meta.put(
          { ...list, ...applyToList(list, change, me.id) },
          "list",
        );
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
      } else if (change.type === "deleteBoard") {
        await boards.delete(change.boardId);
      } else if ("boardId" in change) {
        const board = await boards.get(change.boardId);
        if (board) await boards.put(withChange(board, change));
      }
      await tx.done;
    },

    all: () => loadPending(me.id),

    async remove(seq) {
      await (await open(me.id)).delete("pending", seq);
    },

    // Discards a change, and marks what it touched as out of date.
    async drop({ seq, change }) {
      const db = await open(me.id);
      const tx = db.transaction(["pending", "meta", "boards"], "readwrite");
      await tx.objectStore("pending").delete(seq);
      if (!isBoardChange(change)) {
        const meta = tx.objectStore("meta");
        const list = (await meta.get("list")) as SavedList | undefined;
        if (list) await meta.put({ ...list, outOfDate: true }, "list");
      }
      if ("boardId" in change) {
        const boards = tx.objectStore("boards");
        const board = await boards.get(change.boardId);
        if (board) await boards.put({ ...board, outOfDate: true });
      }
      await tx.done;
    },
  };
}

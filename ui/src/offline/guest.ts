import type { Me } from "@/pages/Boards/loaders";

import type { BoardList, Change } from "./changes";
import {
  deleteCopy,
  loadList,
  loadPending,
  queueAll,
  saveList,
  type PendingChange,
} from "./copy";
import { setGuestState } from "./session";

// Someone using Skipli without an account. Their boards are an Offline copy
// under this id that is never sent; nothing outside it refers to the id, so
// it is gone once the boards have moved into an account.
export const GUEST: Me = { id: "guest", email: "", name: "Guest" };

// Starts a guest's Offline copy with an empty board list, so boards made as
// a guest are listed (queueing a change only updates a list that exists).
export async function startGuestCopy() {
  if (!(await loadList(GUEST.id))) {
    await saveList(GUEST.id, { boards: [], teamBoards: [] });
  }
}

type Queued = { id: string; change: Change };

// A guest's changes as they go to an account, which may have boards of its
// own already. The server saves a board order as positions from 0 for the
// boards it names, so a guest's order would interleave the guest's boards
// with the account's. Instead the guest's boards are created first, in the
// guest's final order: a new board goes after the account's, and each after
// the one before. Favorites are likewise favorited again in the bar's
// order, each going last. Changes to boards the guest deleted are left out.
export function forAccount(
  pending: PendingChange[],
  list: BoardList | undefined,
): Queued[] {
  const boards = list?.boards ?? [];
  const position = new Map(boards.map((board, i) => [board.id, i]));
  const kept = pending.filter(
    ({ change }) =>
      change.type !== "orderBoards" &&
      change.type !== "orderFavorites" &&
      change.type !== "favoriteBoard" &&
      position.has(change.boardId),
  );

  const creates = kept
    .filter(({ change }) => change.type === "createBoard")
    .sort(
      (a, b) =>
        position.get(boardIdOf(a.change))! - position.get(boardIdOf(b.change))!,
    );
  const rest = kept.filter(({ change }) => change.type !== "createBoard");

  // Each under the id of the guest's own last favoriting of it, so moving
  // twice queues it once.
  const favorites = boards
    .filter((board) => board.favorite)
    .sort((a, b) => (a.favoriteOrder ?? 0) - (b.favoriteOrder ?? 0))
    .flatMap((board) => {
      const last = [...pending]
        .reverse()
        .find(
          ({ change }) =>
            change.type === "favoriteBoard" && change.boardId === board.id,
        );
      if (!last) return [];
      return [
        {
          id: last.id,
          change: {
            type: "favoriteBoard",
            boardId: board.id,
            favorite: true,
          } as const,
        },
      ];
    });

  return [
    ...creates.map(({ id, change }) => ({ id, change })),
    ...rest.map(({ id, change }) => ({ id, change })),
    ...favorites,
  ];
}

const boardIdOf = (change: Change) =>
  "boardId" in change ? change.boardId : "";

// Moves a guest's boards into the account just signed in to: their changes
// join the account's Pending changes, to be sent as changes made offline
// are on reconnect, and the guest's copy is deleted. Queued as the account,
// the boards are its own in the Offline copy at once, as they will be on
// the server, which makes whoever creates a board its owner.
export async function moveGuestInto(me: Me) {
  const [pending, list] = await Promise.all([
    loadPending(GUEST.id),
    loadList(GUEST.id),
  ]);
  await queueAll(me, forAccount(pending, list));
  await deleteCopy(GUEST.id);
  setGuestState(null);
}

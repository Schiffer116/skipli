import { isBoardChange, type Change } from "@/offline/changes";
import {
  forgetBoard,
  loadPending,
  saveBoard,
  type SavedBoard,
} from "@/offline/copy";
import { apiFetch } from "@/utils/apiFetch";
import { throwIfNotOk } from "@/utils/throwIfNotOk";

import { apply } from "./apply";

// A board as GET /api/boards/{id} answers it: all of it, in one request.
export type FetchedBoard = Omit<SavedBoard, "savedAt">;

// Reads a whole board from the server in one request, lays this person's
// Pending changes to it on top (the server doesn't have those yet), and
// keeps the result in the Offline copy. A board that's gone, or no longer
// theirs, leaves the Offline copy; that answer, like any other failure, is
// thrown as loaders throw it.
export async function fetchBoard(
  userId: string,
  boardId: string,
): Promise<FetchedBoard> {
  const res = await apiFetch(`/api/boards/${encodeURIComponent(boardId)}`);
  if (res.status === 403 || res.status === 404) {
    await forgetBoard(userId, boardId);
  }
  throwIfNotOk(res);
  const fetched: FetchedBoard = await res.json();

  const pending = await loadPending(userId);
  const board = pending.reduce(
    (board, { change }) => withChange(board, change),
    fetched,
  );
  await saveBoard(userId, board);
  return board;
}

function withChange(board: FetchedBoard, change: Change): FetchedBoard {
  if (!("boardId" in change) || change.boardId !== board.id) return board;
  if (isBoardChange(change)) {
    return { ...board, cards: apply(board.cards, change) };
  }
  if (change.type === "updateBoard") {
    return { ...board, name: change.name, description: change.description };
  }
  return board;
}

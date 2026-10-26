import type { Board } from "@/pages/Boards/BoardList/BoardList";

// Every change the app can make, in the shape the API takes. Online, a
// change is sent right away; offline it waits as a Pending change and is
// sent in order on reconnect. An update carries only the fields it changed,
// so teammates' edits to the others survive; the last to arrive wins per
// field.
export type Change =
  | { type: "createBoard"; boardId: string; name: string; description: string }
  | {
      type: "updateBoard";
      boardId: string;
      name?: string;
      description?: string;
    }
  | { type: "deleteBoard"; boardId: string }
  | { type: "favoriteBoard"; boardId: string; favorite: boolean }
  | { type: "orderBoards"; boardIds: string[] }
  | { type: "orderFavorites"; boardIds: string[] }
  | {
      type: "createCard";
      boardId: string;
      cardId: string;
      name: string;
      createdAt: string;
    }
  | {
      type: "updateCard";
      boardId: string;
      cardId: string;
      name?: string;
      description?: string;
    }
  | { type: "deleteCard"; boardId: string; cardId: string }
  | {
      type: "moveCard";
      boardId: string;
      cardId: string;
      beforeId: string | null;
      afterId: string | null;
    }
  | {
      type: "createTask";
      boardId: string;
      cardId: string;
      taskId: string;
      name: string;
    }
  | {
      type: "updateTask";
      boardId: string;
      // The card the task was in when this change was made. A teammate may
      // have moved it since; the task is found by its id wherever it is.
      cardId: string;
      taskId: string;
      name?: string;
      description?: string;
      status?: string;
    }
  | { type: "deleteTask"; boardId: string; cardId: string; taskId: string }
  | {
      type: "moveTask";
      boardId: string;
      // As in updateTask.
      cardId: string;
      taskId: string;
      newCardId: string;
      beforeId: string | null;
      afterId: string | null;
    }
  // Deletes the tasks done on screen when it was made, and only those.
  | { type: "cleanUp"; boardId: string; taskIds: string[] };

// A Board change: one edit to the cards or tasks of a board.
export type BoardChange = Extract<
  Change,
  {
    type:
      | "createCard"
      | "updateCard"
      | "deleteCard"
      | "moveCard"
      | "createTask"
      | "updateTask"
      | "deleteTask"
      | "moveTask"
      | "cleanUp";
  }
>;

const boardChanges = new Set<Change["type"]>([
  "createCard",
  "updateCard",
  "deleteCard",
  "moveCard",
  "createTask",
  "updateTask",
  "deleteTask",
  "moveTask",
  "cleanUp",
]);

export const isBoardChange = (change: Change): change is BoardChange =>
  boardChanges.has(change.type);

// A change is one HTTP request: a Board change goes to its board's change
// endpoint, anything else to its own route.
export function toRequest(change: Change): { url: string; init: RequestInit } {
  const json = (method: string, body: unknown) => ({
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const board = `/api/boards/${"boardId" in change ? change.boardId : ""}`;

  if (isBoardChange(change)) {
    return { url: `${board}/changes`, init: json("POST", change) };
  }

  switch (change.type) {
    case "createBoard":
      return {
        url: "/api/boards",
        init: json("POST", {
          id: change.boardId,
          name: change.name,
          description: change.description,
        }),
      };
    case "updateBoard":
      return {
        url: board,
        init: json("PUT", fields(change)),
      };
    case "deleteBoard":
      return { url: board, init: { method: "DELETE" } };
    case "favoriteBoard":
      return {
        url: `${board}/favorite`,
        init: json("PUT", { favorite: change.favorite }),
      };
    case "orderBoards":
      return {
        url: "/api/boards/order",
        init: json("PUT", { boardIds: change.boardIds }),
      };
    case "orderFavorites":
      return {
        url: "/api/boards/favorites/order",
        init: json("PUT", { boardIds: change.boardIds }),
      };
  }
}

// The boards, cards and tasks a change touches, for marking what hasn't
// reached the server yet.
export function touchedIds(change: Change): string[] {
  switch (change.type) {
    case "orderBoards":
    case "orderFavorites":
      return [];
    case "createTask":
    case "updateTask":
    case "deleteTask":
    case "moveTask":
      return [change.boardId, change.taskId];
    case "createCard":
    case "updateCard":
    case "deleteCard":
    case "moveCard":
      return [change.boardId, change.cardId];
    default:
      return [change.boardId];
  }
}

// A create that the server already has (it answered 409 to a create sent
// with the same ID) was saved by an earlier attempt that lost its answer.
export function alreadySaved(change: Change, status: number) {
  return status === 409 && change.type.startsWith("create");
}

// The fields an update changed: its name, description or status, if
// given.
export function fields(change: {
  name?: string;
  description?: string;
  status?: string;
}) {
  const { name, description, status } = change;
  return Object.fromEntries(
    Object.entries({ name, description, status }).filter(
      ([, value]) => value !== undefined,
    ),
  ) as { name?: string; description?: string; status?: string };
}

// The fields of now that differ from was: what an update carries.
export function changedFields<T extends Record<string, string>>(
  was: Record<keyof T, unknown>,
  now: T,
): Partial<T> {
  return Object.fromEntries(
    Object.entries(now).filter(([key, value]) => value !== was[key]),
  ) as Partial<T>;
}

export type BoardList = { boards: Board[]; teamBoards: Board[] };

// Applies a board-list change to the offline copy's list. Changes inside a
// board are kept by saving the board as it is on screen instead.
export function applyToList(
  list: BoardList,
  change: Change,
  me: string,
): BoardList {
  const each = (map: (boards: Board[]) => Board[]) => ({
    boards: map(list.boards),
    teamBoards: map(list.teamBoards),
  });

  switch (change.type) {
    case "createBoard":
      return {
        ...list,
        boards: [
          ...list.boards,
          {
            id: change.boardId,
            name: change.name,
            description: change.description,
            owner: me,
            favorite: false,
            taskCount: 0,
            doneCount: 0,
          },
        ],
      };
    case "updateBoard":
      return each((boards) =>
        boards.map((board) =>
          board.id === change.boardId ? { ...board, ...fields(change) } : board,
        ),
      );
    case "deleteBoard":
      return each((boards) =>
        boards.filter((board) => board.id !== change.boardId),
      );
    case "favoriteBoard": {
      // A new favorite goes last in the bar, as the server puts it.
      const last = Math.max(
        0,
        ...[...list.boards, ...list.teamBoards].map(
          (board) => board.favoriteOrder ?? 0,
        ),
      );
      return each((boards) =>
        boards.map((board) =>
          board.id === change.boardId
            ? {
                ...board,
                favorite: change.favorite,
                favoriteOrder: change.favorite ? last + 1 : 0,
              }
            : board,
        ),
      );
    }
    case "orderFavorites": {
      const position = new Map(change.boardIds.map((id, i) => [id, i + 1]));
      return each((boards) =>
        boards.map((board) =>
          position.has(board.id)
            ? { ...board, favoriteOrder: position.get(board.id) }
            : board,
        ),
      );
    }
    case "orderBoards": {
      // The order covers one section of the list; boards it doesn't name
      // (say, made since) keep their place after the ones it does.
      const position = new Map(change.boardIds.map((id, i) => [id, i]));
      const at = (board: Board) =>
        position.get(board.id) ?? change.boardIds.length;
      const reorder = (boards: Board[]) =>
        boards.some((board) => position.has(board.id))
          ? [...boards].sort((a, b) => at(a) - at(b))
          : boards;
      return each(reorder);
    }
    default:
      return list;
  }
}

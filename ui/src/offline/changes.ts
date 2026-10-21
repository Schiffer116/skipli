import type { Board } from "@/pages/Boards/BoardList/BoardList";

// Every change the app can make, in the shape the API takes. Online, a
// change is sent right away; offline (on a Solo board, or to the board
// list) it waits as a Pending change and is sent in order on reconnect.
export type Change =
  | { type: "createBoard"; boardId: string; name: string; description: string }
  | { type: "updateBoard"; boardId: string; name: string; description: string }
  | { type: "deleteBoard"; boardId: string }
  | { type: "favoriteBoard"; boardId: string; favorite: boolean }
  | { type: "orderBoards"; boardIds: string[] }
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
      name: string;
      description: string;
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
      cardId: string;
      taskId: string;
      name: string;
      description: string;
      status: string;
    }
  | { type: "deleteTask"; boardId: string; cardId: string; taskId: string }
  | {
      type: "moveTask";
      boardId: string;
      // The card the task is in on the server when this change is sent.
      cardId: string;
      taskId: string;
      newCardId: string;
      beforeId: string | null;
      afterId: string | null;
    }
  | { type: "cleanUp"; boardId: string };

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
        init: json("PUT", {
          name: change.name,
          description: change.description,
        }),
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
  }
}

// The boards, cards and tasks a change touches, for marking what hasn't
// reached the server yet.
export function touchedIds(change: Change): string[] {
  switch (change.type) {
    case "orderBoards":
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
          board.id === change.boardId
            ? { ...board, name: change.name, description: change.description }
            : board,
        ),
      );
    case "deleteBoard":
      return each((boards) =>
        boards.filter((board) => board.id !== change.boardId),
      );
    case "favoriteBoard":
      return each((boards) =>
        boards.map((board) =>
          board.id === change.boardId
            ? { ...board, favorite: change.favorite }
            : board,
        ),
      );
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

import type { BoardChange, Change } from "@/offline/changes";
import { updateBoard } from "@/offline/copy";
import { onAccepted, refused, submit } from "@/offline/sync";
import type { CardType } from "@/pages/Boards/Board/Card";
import { socket as liveSocket } from "@/utils/socket";

import { apply } from "./apply";

type Socket = Pick<
  typeof liveSocket,
  "connect" | "disconnect" | "emit" | "on" | "off"
>;

export type OpenBoard = {
  cards: () => CardType[];
  subscribe: (listener: () => void) => () => void;
  make: (change: BoardChange) => Promise<void>;
  // Goes live: teammates' changes come in, and accepted ones go out to them.
  start: () => () => void;
};

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

const isBoardChange = (change: Change): change is BoardChange =>
  boardChanges.has(change.type);

// The open board's cards, and the one way to change them. A Board change
// shows on screen first, then goes to the server (or waits as a Pending
// change), and reaches teammates once the server accepts it. What's on
// screen is kept in the Offline copy too.
export function openBoard(
  userId: string,
  boardId: string,
  loaded: CardType[],
  socket: Socket = liveSocket,
): OpenBoard {
  let cards = loaded;
  const listeners = new Set<() => void>();

  // Saves are batched while changes come quickly, and the last one is
  // never lost when the board closes.
  let unsaved = false;
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  const save = () => {
    clearTimeout(saveTimer);
    if (!unsaved) return;
    unsaved = false;
    void updateBoard(userId, boardId, { cards });
  };

  const show = (change: BoardChange) => {
    const next = apply(cards, change);
    if (next === cards) return;
    cards = next;
    unsaved = true;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 300);
    listeners.forEach((listener) => listener());
  };

  const receive = (change: BoardChange) => {
    if (change.boardId === boardId) show(change);
  };

  return {
    cards: () => cards,

    subscribe(listener) {
      listeners.add(listener);
      return () => void listeners.delete(listener);
    },

    async make(change) {
      show(change);
      if (await submit(change)) return;
      // The server has the board otherwise: what's on screen is about to
      // be replaced, so it mustn't overwrite the Offline copy.
      unsaved = false;
      clearTimeout(saveTimer);
      refused();
    },

    start() {
      socket.connect(boardId);
      socket.on("change", receive);
      const stopRelaying = onAccepted((change) => {
        if (isBoardChange(change) && change.boardId === boardId) {
          socket.emit("change", change);
        }
      });
      return () => {
        stopRelaying();
        socket.off("change", receive);
        socket.disconnect();
        save();
      };
    },
  };
}

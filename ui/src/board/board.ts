import type { BoardChange } from "@/offline/changes";
import { loadPending, updateBoard } from "@/offline/copy";
import { refused, settled, submit } from "@/offline/sync";
import type { CardType } from "@/pages/Boards/Board/Card";
import { socket } from "@/utils/socket";

import { apply } from "./apply";

export type OpenBoard = {
  cards: () => CardType[];
  subscribe: (listener: () => void) => () => void;
  make: (change: BoardChange) => Promise<void>;
  // Goes live: opens the board's socket, which brings teammates' changes in.
  start: () => () => void;
};

// A refetch is started again when a change was made while it was loading,
// up to this many times.
const REFETCHES = 3;

// The open board's cards, and the one way to change them. A Board change
// shows on screen first, then goes to the server over HTTP (or waits as a
// Pending change), which broadcasts it to teammates once it accepts it.
// Teammates' changes come in over the board's socket, and the whole board
// is fetched again with reload whenever that socket opens, to catch up on
// anything saved while it wasn't. What's on screen is kept in the Offline
// copy too.
export function openBoard(
  userId: string,
  boardId: string,
  loaded: CardType[],
  reload: () => Promise<CardType[]>,
): OpenBoard {
  let cards = loaded;
  const listeners = new Set<() => void>();

  // Counts what has been shown, so a reload can tell the board changed under it.
  let shown = 0;

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

  const set = (next: CardType[]) => {
    cards = next;
    shown++;
    unsaved = true;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 300);
    listeners.forEach((listener) => listener());
  };

  const show = (change: BoardChange) => {
    const next = apply(cards, change);
    if (next !== cards) set(next);
  };

  const receive = (change: BoardChange) => {
    if (change.boardId === boardId) show(change);
  };

  let live = false;

  // Replaces the cards with the server's, with this device's Pending
  // changes laid on top. Waits for the changes made so far to reach the
  // server first, and starts over if more are made while it loads.
  const catchUp = async () => {
    for (let attempt = 0; attempt < REFETCHES && live; attempt++) {
      await settled();
      const before = shown;
      let fresh: CardType[];
      try {
        fresh = await reload();
      } catch {
        return; // The next time the socket opens will try again.
      }
      const pending = (await loadPending(userId)).filter(
        ({ change }) => "boardId" in change && change.boardId === boardId,
      );
      if (!live || shown !== before) continue;
      set(
        pending.reduce(
          (cards, { change }) => apply(cards, change as BoardChange),
          fresh,
        ),
      );
      return;
    }
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
      live = true;
      const leave = socket.join(boardId);
      const stopReceiving = socket.onChange(receive);
      const stopCatchingUp = socket.onOpen((opened) => {
        if (opened === boardId) void catchUp();
      });
      return () => {
        live = false;
        stopCatchingUp();
        stopReceiving();
        leave();
        save();
      };
    },
  };
}

import { useEffect, useState, useSyncExternalStore } from "react";

import { loadBoard } from "@/offline/copy";
import { useMe } from "@/pages/Boards/favorites";
import type { CardType } from "@/pages/Boards/Board/Card";

import { openBoard } from "./board";
import { fetchBoard, type FetchedBoard } from "./fetch";

// The open board's cards and the way to change them, for as long as the
// board stays open. Each time its socket opens, the whole board loads again
// (once no longer held, during a drag): the cards show here, and the rest is
// passed to onFresh, or "gone" when the board was deleted or is no longer
// this person's.
export default function useOpenBoard(
  boardId: string,
  {
    cards: loaded,
    held,
    onFresh,
  }: {
    cards: CardType[];
    held: boolean;
    onFresh: (board: FetchedBoard | "gone") => void;
  },
) {
  const userId = useMe().id;
  const [board] = useState(() =>
    openBoard(userId, boardId, loaded, async () => {
      try {
        const fresh = await fetchBoard(userId, boardId);
        onFresh(fresh);
        return fresh.cards;
      } catch (error) {
        // A board that's gone has left the Offline copy.
        if (!(await loadBoard(userId, boardId))) onFresh("gone");
        throw error;
      }
    }),
  );
  useEffect(() => board.start(), [board]);
  useEffect(() => board.hold(held), [board, held]);
  const cards = useSyncExternalStore(board.subscribe, board.cards);
  return { cards, make: board.make };
}

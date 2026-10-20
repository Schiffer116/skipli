import { useEffect, useState, useSyncExternalStore } from "react";

import { useMe } from "@/pages/Boards/favorites";
import type { CardType } from "@/pages/Boards/Board/Card";

import { openBoard } from "./board";

// The open board's cards and the way to change them, for as long as the
// board stays open.
export default function useOpenBoard(boardId: string, loaded: CardType[]) {
  const userId = useMe().id;
  const [board] = useState(() => openBoard(userId, boardId, loaded));
  useEffect(() => board.start(), [board]);
  const cards = useSyncExternalStore(board.subscribe, board.cards);
  return { cards, make: board.make };
}

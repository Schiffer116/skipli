import { useEffect, useRef } from "react";

import type { CardType } from "@/pages/Boards/Board/Card";

import { updateBoard } from "./copy";

// Keeps the Offline copy of the open board in step with what's on screen,
// online (including teammates' live changes) and offline alike. Saves are
// batched while things change quickly, such as during a drag, and the last
// one is never lost when the board closes.
export default function useSaveCards(
  userId: string,
  boardId: string,
  cards: CardType[],
) {
  const loaded = useRef(cards);
  const latest = useRef<CardType[] | null>(null);

  useEffect(() => {
    // What the loader just loaded is already saved.
    if (cards === loaded.current) return;
    latest.current = cards;
    const timer = setTimeout(() => {
      latest.current = null;
      void updateBoard(userId, boardId, { cards });
    }, 300);
    return () => clearTimeout(timer);
  }, [userId, boardId, cards]);

  useEffect(
    () => () => {
      if (latest.current)
        void updateBoard(userId, boardId, { cards: latest.current });
    },
    [userId, boardId],
  );
}

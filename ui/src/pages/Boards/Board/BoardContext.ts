import { createContext } from "react";

import type { CardType } from "./Card";

type BoardContextType = {
  cards: CardType[];
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>;
  showCreateCardForm: boolean;
  setShowCreateCardForm: React.Dispatch<React.SetStateAction<boolean>>;
  createTaskFormId: string | null;
  setCreateTaskFormId: React.Dispatch<React.SetStateAction<string | null>>;
};

export const BoardContext = createContext<BoardContextType | null>(null);

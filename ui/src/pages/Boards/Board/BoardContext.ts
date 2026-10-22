import { createContext } from "react";

import type { BoardChange } from "@/offline/changes";

type BoardContextType = {
  // Makes a Board change: the only way the board's cards change.
  make: (change: BoardChange) => Promise<void>;
  showCreateCardForm: boolean;
  setShowCreateCardForm: React.Dispatch<React.SetStateAction<boolean>>;
  createTaskFormId: string | null;
  setCreateTaskFormId: React.Dispatch<React.SetStateAction<string | null>>;
};

export const BoardContext = createContext<BoardContextType | null>(null);

import { useState } from "react";
import { useLoaderData, useRevalidator } from "react-router";

import { fromServer, type BoardViewData } from "@/pages/Boards/loaders";

import type { FetchedBoard } from "./fetch";

// The open board as loaded, or as the server has it once it loads again
// (onFresh). A board that turns out to be gone loads again, to say so.
export default function useFreshBoard() {
  const loaded = useLoaderData<BoardViewData>();
  const { revalidate } = useRevalidator();
  const [fresh, setFresh] = useState<{
    loaded: BoardViewData;
    board: BoardViewData;
  } | null>(null);

  const onFresh = (board: FetchedBoard | "gone") => {
    if (board === "gone") void revalidate();
    else setFresh({ loaded, board: fromServer(board) });
  };
  // Loading the page again starts over from what it loaded.
  const board = fresh?.loaded === loaded ? fresh.board : loaded;
  return { board, onFresh };
}

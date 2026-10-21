import { useLoaderData, useParams } from "react-router";

import useFreshBoard from "@/board/useFreshBoard";
import { revisionOf } from "@/utils/revision";

import Board from "./Board/Board";
import Sidebar from "./Board/Sidebar";

export default function BoardView() {
  const { boardId } = useParams();
  const revision = revisionOf(useLoaderData());
  const { board, onFresh } = useFreshBoard();

  // Keyed by board so switching boards (e.g. from the favorites bar) remounts
  // and re-seeds state copied from the loader, like cards and members.
  return (
    <div key={`${boardId}:${revision}`} className="flex h-full min-w-0">
      <Sidebar members={board.members} />
      <Board board={board} onFresh={onFresh} />
    </div>
  );
}

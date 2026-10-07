import { useParams } from "react-router";

import Board from "./Board/Board";
import Sidebar from "./Board/Sidebar";

export default function BoardView() {
  const { boardId } = useParams();

  // Keyed by board so switching boards (e.g. from the favorites bar) remounts
  // and re-seeds state copied from the loader, like cards and members.
  return (
    <div key={boardId} className="flex h-[calc(100vh-5.25rem)] min-w-0">
      <Sidebar />
      <Board />
    </div>
  );
}

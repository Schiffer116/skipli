import { useLoaderData, useParams } from "react-router";

import { revisionOf } from "@/utils/revision";

import Board from "./Board/Board";
import Sidebar from "./Board/Sidebar";

export default function BoardView() {
  const { boardId } = useParams();
  const revision = revisionOf(useLoaderData());

  // Keyed by board so switching boards (e.g. from the favorites bar) remounts
  // and re-seeds state copied from the loader, like cards and members.
  return (
    <div key={`${boardId}:${revision}`} className="flex h-full min-w-0">
      <Sidebar />
      <Board />
    </div>
  );
}

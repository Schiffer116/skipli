import { Plus } from "lucide-react";
import { useEffect, useState } from "react";
import { useLoaderData, useRevalidator } from "react-router";

import { Button } from "@/components/ui/button";
import { useSync } from "@/offline/sync";

import { revisionOf } from "@/utils/revision";

import CreateBoardForm from "./CreateBoardForm";
import SortableBoards from "./SortableBoards";
import type { BoardListData } from "../loaders";

export type Board = {
  id: string;
  name: string;
  owner: string;
  description: string;
  favorite: boolean;
  taskCount: number;
  doneCount: number;
};

// Keyed by the loader's data, so reloading it starts the list afresh.
export default function BoardListRoute() {
  const loaderData = useLoaderData<BoardListData>();
  useReloadOnConnectionChange(loaderData);
  return <BoardList key={revisionOf(loaderData)} loaderData={loaderData} />;
}

// Which boards are listed and which can be changed differ offline, so the
// list loads again when the connection comes or goes. Changes made from the
// list are already in the Offline copy, so nothing on screen is lost.
function useReloadOnConnectionChange(loaderData: BoardListData) {
  const { offline } = useSync();
  const { revalidate, state } = useRevalidator();
  const loadedOffline = loaderData.offline !== undefined;

  useEffect(() => {
    if (offline !== loadedOffline && state === "idle") void revalidate();
  }, [offline, loadedOffline, state, revalidate]);
}

function BoardList({ loaderData }: { loaderData: BoardListData }) {
  const offline = loaderData.offline;
  const [boards, setBoards] = useState(loaderData.boards);
  const [teamBoards, setTeamBoards] = useState(loaderData.teamBoards);
  const [showCreateBoardForm, setShowCreateBoardForm] = useState(false);

  return (
    <div className="flex flex-col gap-10 px-10 py-8">
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-bold">Your workspace</h1>
        <SortableBoards
          boards={boards}
          setBoards={setBoards}
          offline={offline}
          editable
        >
          {showCreateBoardForm ? (
            <CreateBoardForm
              setBoards={setBoards}
              setShowCreateBoardForm={setShowCreateBoardForm}
            />
          ) : (
            <Button
              variant="ghost"
              className="max-w-xs h-full aspect-video w-full rounded-lg border-2 border-dashed border-border bg-card/60 hover:border-link hover:bg-accent text-muted-foreground hover:text-link"
              onClick={() => setShowCreateBoardForm(true)}
            >
              <Plus className="h-5 w-5" />
              Create new board
            </Button>
          )}
        </SortableBoards>
      </div>

      {teamBoards.length > 0 && (
        <div className="flex flex-col gap-4">
          <h2 className="text-xl font-bold">Your teams</h2>
          <SortableBoards
            boards={teamBoards}
            setBoards={setTeamBoards}
            offline={offline}
          />
        </div>
      )}

      {offline?.missing && (
        <p className="text-sm text-muted-foreground">
          Some boards aren't available offline. Boards you open while online are
          kept on this device.
        </p>
      )}
    </div>
  );
}

import { Plus } from "lucide-react";
import { useState } from "react";
import { redirect } from "react-router";
import { useLoaderData } from "react-router";

import { Button } from "@/components/ui/button";

import CreateBoardForm from "./CreateBoardForm";
import SortableBoards from "./SortableBoards";

export type Board = {
  id: string;
  name: string;
  owenr: string;
  description: string;
};

export const boardListLoader = async (): Promise<{
  boards: Board[];
  teamBoards: Board[];
}> => {
  const res = await fetch("/api/boards", {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  if (!res.ok) {
    throw redirect("/login");
  }
  const { boards, teamBoards } = await res.json();
  const boardIds = boards.map((board: Board) => board.id);

  console.log("board", boards);

  return {
    boards,
    teamBoards: teamBoards.filter(
      (board: Board) => !boardIds.includes(board.id),
    ),
  };
};

export default function BoardList() {
  const loaderData = useLoaderData<typeof boardListLoader>();
  const [boards, setBoards] = useState(loaderData.boards);
  const [teamBoards, setTeamBoards] = useState(loaderData.teamBoards);
  const [showCreateBoardForm, setShowCreateBoardForm] = useState(false);

  return (
    <div className="flex flex-col gap-10 px-10 py-8">
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-bold">Your workspace</h1>
        <SortableBoards boards={boards} setBoards={setBoards} editable>
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

      <div className="flex flex-col gap-4">
        <h2 className="text-xl font-bold">Your teams</h2>
        <SortableBoards boards={teamBoards} setBoards={setTeamBoards} />
      </div>
    </div>
  );
}

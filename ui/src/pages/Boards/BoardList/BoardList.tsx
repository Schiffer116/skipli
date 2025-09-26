import { Plus } from "lucide-react";
import { useState } from "react";
import { redirect, useNavigate } from "react-router";
import { useLoaderData } from "react-router";

import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";

import CreateBoardForm from "./CreateBoardForm";
import EditBoardDialog from "./EditBoardDialog";

export type Board = {
  id: string;
  name: string;
  description: string;
};

export const boardListLoader = async (): Promise<{
  boards: Board[];
  teamBoards: Board[];
}> => {
  const data = await fetch("/api/boards", {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  if (!data.ok) {
    throw redirect("/login");
  }
  const { boards, teamBoards } = await data.json();
  const boardIds = boards.map((board: Board) => board.id);

  return {
    boards,
    teamBoards: teamBoards.filter(
      (board: Board) => !boardIds.includes(board.id),
    ),
  };
};

export default function BoardList() {
  const navigate = useNavigate();

  const [boards, setBoards] = useState(
    useLoaderData<typeof boardListLoader>().boards,
  );
  const teamBoards = useLoaderData<typeof boardListLoader>().teamBoards;

  useLoaderData<typeof boardListLoader>();
  const [showCreateBoardForm, setShowCreateBoardForm] = useState(false);

  return (
    <div className="flex flex-col gap-16 p-12">
      <div className="flex flex-col gap-8">
        <h1 className="text-3xl font-bold text-primary">Your workspace</h1>
        <div className="flex flex-wrap items-start gap-8">
          {boards.map((board) => (
            <Card
              key={board.id}
              className="relative w-full max-w-xs aspect-video cursor-pointer p-2"
              onClick={() => navigate(`/boards/${board.id}`)}
            >
              <CardHeader className="flex justify-between items-center p-0 pl-2">
                <CardTitle>{board.name}</CardTitle>
                <EditBoardDialog {...board} setBoards={setBoards} />
              </CardHeader>
            </Card>
          ))}

          {showCreateBoardForm ? (
            <CreateBoardForm
              setBoards={setBoards}
              setShowCreateBoardForm={setShowCreateBoardForm}
            />
          ) : (
            <Button
              variant="ghost"
              className="max-w-xs h-full aspect-video w-full border-2 border-dashed border-border hover:border-primary hover:bg-accent/50 text-muted-foreground hover:text-primary"
              onClick={() => setShowCreateBoardForm(true)}
            >
              <Plus className="h-5 w-5" />
              Create new board
            </Button>
          )}
        </div>
      </div>

      <div className="flex flex-col gap-8">
        <h2 className="text-3xl font-bold text-primary">Your teams</h2>
        <div className="flex flex-wrap items-start gap-8">
          {teamBoards.map((board) => (
            <Card
              key={board.id}
              className="relative w-full max-w-xs aspect-video cursor-pointer p-2"
              onClick={() => navigate(`/boards/${board.id}`)}
            >
              <CardHeader className="flex justify-between items-center p-2">
                <CardTitle>{board.name}</CardTitle>
              </CardHeader>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
}

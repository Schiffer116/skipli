import { BrushCleaning } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { useContext } from "react";

import type { CardType } from "./Card/Card";
import { BoardContext } from "./BoardContext";

type CleanUpButtonProps = {
  boardId: string;
  cards: CardType[];
};

export default function CleanUpButton({
  boardId,
  cards,
}: CleanUpButtonProps) {
  const { make } = useContext(BoardContext)!;
  const doneCount = cards
    .flatMap((card) => card.tasks)
    .filter((task) => task.status === "done").length;

  const cleanUp = () => {
    void make({ type: "cleanUp", boardId });
  };

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" disabled={doneCount === 0}>
          <BrushCleaning />
          Clean up
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Clean up this board?</AlertDialogTitle>
          <AlertDialogDescription>
            This permanently deletes {doneCount} finished{" "}
            {doneCount === 1 ? "task" : "tasks"} from every card on the board.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={cleanUp}>Delete</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

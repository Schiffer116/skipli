import { Sparkles } from "lucide-react";

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
import { apiFetch } from "@/utils/apiFetch";

import type { CardType } from "./Card/Card";

type CleanUpButtonProps = {
  boardId: string;
  cards: CardType[];
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>;
};

export default function CleanUpButton({
  boardId,
  cards,
  setCards,
}: CleanUpButtonProps) {
  const doneCount = cards
    .flatMap((card) => card.tasks)
    .filter((task) => task.status === "done").length;

  const cleanUp = async () => {
    const res = await apiFetch(`/api/boards/${boardId}/cleanup`, {
      method: "POST",
    });
    if (!res.ok) {
      return;
    }

    setCards((cards) =>
      cards.map((card) => ({
        ...card,
        tasks: card.tasks.filter((task) => task.status !== "done"),
      })),
    );
  };

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" disabled={doneCount === 0}>
          <Sparkles />
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

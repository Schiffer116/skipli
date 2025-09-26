import { useContext } from "react";
import { Edit, Trash } from "lucide-react";

import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AlertDialog } from "@radix-ui/react-alert-dialog";
import {
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
import { DialogDescription } from "@radix-ui/react-dialog";

import { socket } from "@/main";
import type { CardType } from "./Card";
import { BoardContext } from "../Board";
import { useParams } from "react-router";
import { deleteCard, updateCard } from "@/utils/updateCard";

export default function EditCardDialog(props: CardType) {
  const { id, name, description } = props;

  const boardId = useParams().boardId!;
  const { setCards, setCreateTaskFormId, setShowCreateCardForm } =
    useContext(BoardContext)!;

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name");
    const description = formData.get("description") ?? "";
    if (!name) {
      return;
    }

    const res = await fetch(`/api/boards/${boardId}/cards/${id}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
      },
      body: JSON.stringify({
        name,
        description,
      }),
    });

    if (!res.ok) {
      return;
    }

    const updatedCard = await res.json();
    socket.emit("update card", { ...updatedCard });
    setCards(updateCard(updatedCard));
  };

  const handleDelete = async () => {
    const res = await fetch(`/api/boards/${boardId}/cards/${id}`, {
      method: "DELETE",
      headers: {
        Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
      },
    });

    if (!res.ok) {
      return;
    }

    socket.emit("delete card", id);
    setCards(deleteCard(id));
  };

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          onClick={(e) => {
            e.stopPropagation();
            setCreateTaskFormId(null);
            setShowCreateCardForm(false);
          }}
        >
          <Edit />
        </Button>
      </DialogTrigger>
      <DialogContent
        className="sm:max-w-[425px]"
        onClick={(e) => e.stopPropagation()}
      >
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Edit card</DialogTitle>
          </DialogHeader>
          <DialogDescription></DialogDescription>

          <div className="grid gap-4 mt-4">
            <div className="grid gap-3">
              <Label htmlFor="board-name">Name</Label>
              <Input id="board-name" name="name" defaultValue={name} />
            </div>
            <div className="grid gap-3">
              <Label htmlFor="board-description">Description</Label>
              <Input
                id="board-description"
                name="description"
                defaultValue={description}
              />
            </div>
          </div>
          <DialogFooter className="mt-4">
            <DialogClose asChild>
              <Button type="submit">Save</Button>
            </DialogClose>
            <DialogClose asChild>
              <Button variant="outline">Cancel</Button>
            </DialogClose>

            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="destructive" className="ml-auto">
                  <Trash className="h-5 w-5" />
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Are you absolutely sure?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This action cannot be undone. This will permanently delete
                    your card and all its tasks
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction asChild>
                    <DialogClose onClick={handleDelete}>Delete</DialogClose>
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

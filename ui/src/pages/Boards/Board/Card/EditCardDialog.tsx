import { useContext, useState } from "react";
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
import { Textarea } from "@/components/ui/textarea";
import { submitOnModEnter } from "@/lib/submitOnModEnter";
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

import type { CardType } from "./Card";
import { BoardContext } from "../BoardContext";
import { useParams } from "react-router";

export default function EditCardDialog(props: CardType) {
  const { id, name, description } = props;

  const boardId = useParams().boardId!;
  const { make, setCreateTaskFormId, setShowCreateCardForm } =
    useContext(BoardContext)!;

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    const description = (formData.get("description") as string) ?? "";
    if (!name) {
      return;
    }

    void make({ type: "updateCard", boardId, cardId: id, name, description });
    setOpen(false);
  };

  const handleDelete = () => {
    void make({ type: "deleteCard", boardId, cardId: id });
  };

  const [open, setOpen] = useState(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Edit card"
          onClick={(e) => {
            e.stopPropagation();
            setCreateTaskFormId(null);
            setShowCreateCardForm(false);
          }}
        >
          <Edit />
        </Button>
      </DialogTrigger>
      {open && (
        <DialogContent
          className="sm:max-w-[425px]"
          onClick={(e) => e.stopPropagation()}
          // Dialogs render in a portal, but React still bubbles events up to
          // the draggable parent; keep a text selection from starting a drag.
          onMouseDown={(e) => e.stopPropagation()}
        >
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>Edit card</DialogTitle>
            </DialogHeader>
            <DialogDescription></DialogDescription>

            <div className="grid gap-4 mt-4">
              <div className="grid gap-3">
                <Label htmlFor="card-name">Name</Label>
                <Input id="card-name" name="name" defaultValue={name} />
              </div>
              <div className="grid gap-3">
                <Label htmlFor="card-description">Description</Label>
                <Textarea
                  id="card-description"
                  name="description"
                  defaultValue={description}
                  onKeyDown={submitOnModEnter}
                />
              </div>
            </div>
            <DialogFooter className="mt-4 flex-row justify-end">
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button
                    type="button"
                    variant="destructive"
                    className="mr-auto"
                    aria-label="Delete card"
                  >
                    <Trash className="h-5 w-5" />
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Are you absolutely sure?
                    </AlertDialogTitle>
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

              <DialogClose asChild>
                <Button type="button" variant="outline">
                  Cancel
                </Button>
              </DialogClose>
              {/* Not a DialogClose: closing unmounts the form before the
                  browser submits it, and the edit is lost. */}
              <Button type="submit">Save</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      )}
    </Dialog>
  );
}

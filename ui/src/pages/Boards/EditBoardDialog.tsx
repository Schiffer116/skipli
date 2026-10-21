import { useState } from "react";
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
import { submit } from "@/offline/sync";

type EditBoardDialogProps = {
  id: string;
  name: string;
  description: string;
  // Called once the change is accepted, to show it.
  onSaved: (name: string, description: string) => void;
  onDeleted: () => void;
};

export default function EditBoardDialog(props: EditBoardDialogProps) {
  const { id, name, description, onSaved, onDeleted } = props;
  const [open, setOpen] = useState(false);

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    const description = formData.get("description") as string;
    if (!name) {
      return;
    }

    setOpen(false);
    if (
      !(await submit({ type: "updateBoard", boardId: id, name, description }))
    ) {
      return;
    }

    onSaved(name, description);
  };

  const deleteBoard = async () => {
    if (!(await submit({ type: "deleteBoard", boardId: id }))) {
      return;
    }

    onDeleted();
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Edit board"
          onClick={(e) => {
            e.stopPropagation();
          }}
        >
          <Edit />
        </Button>
      </DialogTrigger>
      <DialogContent
        className="sm:max-w-[425px]"
        onClick={(e) => e.stopPropagation()}
        // Dialogs render in a portal, but React still bubbles events up to
        // the draggable parent; keep a text selection from starting a drag.
        onMouseDown={(e) => e.stopPropagation()}
      >
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Edit board</DialogTitle>
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
          <DialogFooter className="mt-4 flex-row justify-end">
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  type="button"
                  variant="destructive"
                  className="mr-auto"
                  aria-label="Delete board"
                >
                  <Trash className="h-5 w-5" />
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Are you absolutely sure?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This action cannot be undone. This will permanently delete
                    your board and remove your data from our servers.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction asChild>
                    <DialogClose onClick={deleteBoard}>Delete</DialogClose>
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
    </Dialog>
  );
}

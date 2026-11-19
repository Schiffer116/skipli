import { useContext, useState } from "react";
import { Edit } from "lucide-react";
import { useParams } from "react-router";

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

import { changedFields } from "@/offline/changes";

import { BoardContext } from "../../BoardContext";
import type { TaskType } from "./Task";

export default function EditTaskDialog(props: TaskType) {
  const { id, cardId, name, description } = props;

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

    const changed = changedFields(props, { name, description });
    if (Object.keys(changed).length > 0) {
      void make({
        type: "updateTask",
        boardId,
        cardId,
        taskId: id,
        ...changed,
      });
    }
    setOpen(false);
  };

  const handleDelete = () => {
    void make({ type: "deleteTask", boardId, cardId, taskId: id });
  };

  const [open, setOpen] = useState(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100 transition-colors"
          aria-label="Edit task"
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
        >
          <form onSubmit={handleSubmit} autoComplete="off">
            <DialogHeader>
              <DialogTitle>Edit task</DialogTitle>
            </DialogHeader>
            <DialogDescription></DialogDescription>

            <div className="grid gap-4 mt-4">
              <div className="grid gap-3">
                <Label htmlFor="task-name">Name</Label>
                <Input id="task-name" name="name" defaultValue={name} />
              </div>
              <div className="grid gap-3">
                <Label htmlFor="task-description">Description</Label>
                <Textarea
                  id="task-description"
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
                  >
                    Delete
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Are you absolutely sure?
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      This action cannot be undone. This will permanently delete
                      your task
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={handleDelete}>
                      Delete
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

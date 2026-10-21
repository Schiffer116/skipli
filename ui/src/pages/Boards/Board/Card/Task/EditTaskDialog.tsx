import { useContext, useState } from "react";
import { Edit, Trash } from "lucide-react";
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

import { BoardContext } from "../../BoardContext";
import type { TaskType } from "./Task";

export default function EditTaskDialog(props: TaskType) {
  const { id, cardId, name, description, status } = props;

  const boardId = useParams().boardId!;
  const { make, setCreateTaskFormId, setShowCreateCardForm } =
    useContext(BoardContext)!;

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name") as string;
    const description = (formData.get("description") as string) ?? "";
    const status = (formData.get("status") as string) ?? "";
    if (!name) {
      return;
    }

    void make({
      type: "updateTask",
      boardId,
      cardId,
      taskId: id,
      name,
      description,
      status,
    });
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
                <Input
                  id="task-description"
                  name="description"
                  defaultValue={description}
                />
              </div>
              <div className="grid gap-3">
                <Label htmlFor="task-status">Status</Label>
                <Input id="task-status" name="status" defaultValue={status} />
              </div>
            </div>
            <DialogFooter className="mt-4">
              {/* Not a DialogClose: closing unmounts the form before the
                  browser submits it, and the edit is lost. */}
              <Button type="submit">Save</Button>
              <DialogClose asChild>
                <Button type="button" variant="outline">
                  Cancel
                </Button>
              </DialogClose>

              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button
                    type="button"
                    variant="destructive"
                    className="ml-auto"
                    aria-label="Delete task"
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
            </DialogFooter>
          </form>
        </DialogContent>
      )}
    </Dialog>
  );
}

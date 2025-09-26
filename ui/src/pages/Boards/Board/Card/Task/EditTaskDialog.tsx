import { useContext } from "react";
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

import { socket } from "@/main";
import { BoardContext } from "../../Board";
import type { TaskType } from "./Task";
import { deleteTask, updateTask } from "@/utils/updateCard";

export default function EditTaskDialog(props: TaskType) {
  const { id, cardId, name, description, status } = props;

  const boardId = useParams().boardId!;
  const { setCards, setCreateTaskFormId, setShowCreateCardForm } =
    useContext(BoardContext)!;

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const formData = new FormData(e.currentTarget);
    const name = formData.get("name");
    const description = formData.get("description") ?? "";
    const status = formData.get("status") ?? "";
    if (!name) {
      return;
    }

    const res = await fetch(
      `/api/boards/${boardId}/cards/${cardId}/tasks/${id}`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
        },
        body: JSON.stringify({
          name,
          description,
          status,
        }),
      },
    );

    if (!res.ok) {
      return;
    }

    const updatedTask = await res.json();
    socket.emit("update task", updatedTask);
    setCards(updateTask(updatedTask));
  };

  const handleDelete = async () => {
    const res = await fetch(
      `/api/boards/${boardId}/cards/${cardId}/tasks/${id}`,
      {
        method: "DELETE",
        headers: {
          Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
        },
      },
    );

    if (!res.ok) {
      return;
    }

    socket.emit("delete task", id);
    setCards(deleteTask(id));
  };

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="invisible group-hover:visible"
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
    </Dialog>
  );
}

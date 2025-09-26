import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { forwardRef, useContext } from "react";
import EditTaskDialog from "./EditTaskDialog";
import type { SyntheticListenerMap } from "@dnd-kit/core/dist/hooks/utilities";
import { CheckCircle2, Circle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useParams } from "react-router";
import { socket } from "@/main";
import { updateTask } from "@/utils/updateCard";
import { BoardContext } from "../../Board";

export type TaskType = {
  id: string;
  cardId: string;
  name: string;
  description: string;
  status: string;
};

export async function fetchTasks(boardId: string, cardId: string) {
  const data = await fetch(`/api/boards/${boardId}/cards/${cardId}/tasks`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });
  const tasks: TaskType[] = await data.json();
  return tasks;
}

export const fetchTask = async (
  boardId: string,
  cardId: string,
  taskId: string,
) => {
  const data = await fetch(
    `/api/boards/${boardId}/cards/${cardId}/tasks/${taskId}`,
  );
  const task: TaskType = await data.json();
  return task;
};

export default function Task(props: TaskType) {
  const { id } = props;

  const {
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
    setNodeRef,
  } = useSortable({
    id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <DummyTask
      ref={setNodeRef}
      style={style}
      {...props}
      {...attributes}
      listeners={listeners}
    />
  );
}

type DummyTaskProps = TaskType & {
  listeners?: SyntheticListenerMap;
} & React.HTMLAttributes<HTMLDivElement>;

export const DummyTask = forwardRef<HTMLDivElement, DummyTaskProps>(
  (props, ref) => {
    const { id, name, description, status, listeners, cardId, ...rest } = props;
    const { boardId } = useParams();
    const { setCards } = useContext(BoardContext)!;

    const toggleStatus = async () => {
      const res = await fetch(
        `/api/boards/${boardId}/cards/${cardId}/tasks/${id}`,
        {
          method: "PUT",
          headers: {
            Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            id,
            name,
            description,
            status: status === "done" ? "pending" : "done",
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

    return (
      <Card ref={ref} {...rest} className="h-12 p-0 cursor-pointer">
        <CardHeader className="relative group flex items-center p-0 pr-2 h-full">
          <Button
            className={`${status === "done" ? "block" : "hidden group-hover:block "} `}
            variant="ghost"
            size="icon"
            asChild
            onClick={toggleStatus}
          >
            {status === "done" ? (
              <CheckCircle2 className="w-5 h-5 text-green-500 ml-3" />
            ) : (
              <Circle className="w-5 h-5 text-gray-400 ml-3" />
            )}
          </Button>
          <CardTitle
            className={`h-full ${status === "done" ? "ml-1" : "ml-4"} group-hover:ml-1 flex-1 font-medium flex items-center`}
            {...listeners}
          >
            {name}
          </CardTitle>
          <EditTaskDialog {...props} />
        </CardHeader>
      </Card>
    );
  },
);

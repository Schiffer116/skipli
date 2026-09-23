import { forwardRef, useContext } from "react";
import { Plus } from "lucide-react";

import type { SyntheticListenerMap } from "@dnd-kit/core/dist/hooks/utilities";
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

import { BoardContext } from "../Board";
import Task, { DummyTask, type TaskType } from "./Task";
import CreateTaskForm from "./Task/CreateTaskForm";
import EditCardDialog from "./EditCardDialog";
import { throwIfNotOk } from "@/utils/throwIfNotOk";

export type CardType = {
  id: string;
  name: string;
  description: string;
  tasks: TaskType[];
};

export async function fetchCards(boardId: string) {
  const data = await fetch(`/api/boards/${boardId}/cards`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });
  throwIfNotOk(data);
  const cards: CardType[] = await data.json();
  return cards;
}

export async function fetchCard(boardId: string, cardId: string) {
  const data = await fetch(`/api/boards/${boardId}/cards/${cardId}`);
  const cards: CardType = await data.json();
  return cards;
}

export default function CardView(props: CardType) {
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
    <DummyCard
      ref={setNodeRef}
      style={style}
      {...props}
      {...attributes}
      listeners={listeners}
    />
  );
}

type DummyCardProps = CardType & {
  listeners?: SyntheticListenerMap;
  // Rendered inside DragOverlay: its tasks must not register as sortables,
  // or they'd share ids with the real ones and dnd-kit would re-measure the
  // pair against each other forever.
  overlay?: boolean;
} & React.HTMLAttributes<HTMLDivElement>;

export const DummyCard = forwardRef<HTMLDivElement, DummyCardProps>(
  (props, ref) => {
    const { id, name, description, tasks, listeners, overlay, ...rest } =
      props;
    const { createTaskFormId, setCreateTaskFormId, setShowCreateCardForm } =
      useContext(BoardContext)!;

    return (
      <div ref={ref} {...rest} className="flex-shrink-0 w-80">
        <Card className="bg-card gap-0 py-0 overflow-hidden">
          <CardHeader
            className="cursor-grab flex items-center justify-between px-4 py-3 bg-muted border-b"
            {...listeners}
          >
            <CardTitle className="text-base font-bold text-card-foreground w-full">
              {name}
              <span className="ml-2 font-normal text-muted-foreground">
                ({tasks.length})
              </span>
            </CardTitle>
            <EditCardDialog {...props} />
          </CardHeader>
          <CardContent className={tasks.length ? "px-3 pt-3" : "px-3"}>
            {overlay ? (
              <div className="space-y-2">
                {tasks.map((task) => (
                  <DummyTask key={task.id} {...task} />
                ))}
              </div>
            ) : (
              <SortableContext
                id={id}
                items={tasks}
                strategy={verticalListSortingStrategy}
              >
                <div className="space-y-2">
                  {tasks.map((task) => (
                    <Task key={task.id} {...task} />
                  ))}
                </div>
              </SortableContext>
            )}
          </CardContent>
          <CardFooter className="px-3 py-2">
            {createTaskFormId === id ? (
              <CreateTaskForm cardId={id} />
            ) : (
              <Button
                variant="ghost"
                className="h-9 w-full justify-start rounded-md font-bold text-link hover:text-link hover:bg-accent p-0"
                onClick={(e) => {
                  e.stopPropagation();
                  setCreateTaskFormId(id);
                  setShowCreateCardForm(false);
                }}
              >
                <Plus className="h-4 w-4 mr-2 ml-2" />
                Create a task
              </Button>
            )}
          </CardFooter>
        </Card>
      </div>
    );
  },
);

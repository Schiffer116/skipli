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
import Task, { type TaskType } from "./Task";
import CreateTaskForm from "./Task/CreateTaskForm";
import EditCardDialog from "./EditCardDialog";

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
} & React.HTMLAttributes<HTMLDivElement>;

export const DummyCard = forwardRef<HTMLDivElement, DummyCardProps>(
  (props, ref) => {
    const { id, name, description, tasks, listeners, ...rest } = props;
    const { createTaskFormId, setCreateTaskFormId, setShowCreateCardForm } =
      useContext(BoardContext)!;

    return (
      <div ref={ref} {...rest} className="flex-shrink-0 w-80">
        <Card
          className={`bg-card border-border shadow-sm gap-4 py-4 ${tasks.length == 0 && "gap-2"}`}
        >
          <CardHeader className="cursor-pointer flex items-center justify-between">
            <CardTitle
              className="text-md font-semibold text-card-foreground w-full"
              {...listeners}
            >
              {name}
            </CardTitle>
            <EditCardDialog {...props} />
          </CardHeader>
          <CardContent>
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
          </CardContent>
          <CardFooter>
            {createTaskFormId === id ? (
              <CreateTaskForm cardId={id} />
            ) : (
              <Button
                variant="ghost"
                className="h-12 w-full justify-start text-muted-foreground hover:text-primary hover:bg-accent/50 p-0"
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

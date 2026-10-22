import { forwardRef, memo, useContext } from "react";
import { Plus } from "lucide-react";

import type { SyntheticListenerMap } from "@dnd-kit/core/dist/hooks/utilities";
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { Button } from "@/components/ui/button";
import UnsentMark from "@/components/UnsentMark";
import { usePending } from "@/offline/pending";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

import { BoardContext } from "../BoardContext";
import Task, { DummyTask, type TaskType } from "./Task/Task";
import CreateTaskForm from "./Task/CreateTaskForm";
import EditCardDialog from "./EditCardDialog";

export type CardType = {
  id: string;
  name: string;
  description: string;
  tasks: TaskType[];
};

export default function CardView(props: CardType & { staticTasks?: boolean }) {
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
    transform: CSS.Translate.toString(transform),
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
  // While a card is dragged, its tasks aren't drop targets; skipping their
  // sortables saves re-rendering and re-measuring every task on each move.
  staticTasks?: boolean;
} & React.HTMLAttributes<HTMLDivElement>;

export const DummyCard = forwardRef<HTMLDivElement, DummyCardProps>(
  (props, ref) => {
    const {
      id,
      name,
      description,
      tasks,
      listeners,
      overlay,
      staticTasks,
      ...rest
    } = props;

    return (
      <div
        ref={ref}
        {...rest}
        className={overlay ? "w-full" : "flex-shrink-0 w-80"}
      >
        <CardBody
          id={id}
          name={name}
          description={description}
          tasks={tasks}
          listeners={listeners}
          staticTasks={overlay || staticTasks}
        />
      </div>
    );
  },
);

const CardBody = memo(function CardBody(
  props: CardType & {
    listeners?: SyntheticListenerMap;
    staticTasks?: boolean;
  },
) {
  const { id, name, description, tasks, listeners, staticTasks } = props;
  const { createTaskFormId, setCreateTaskFormId, setShowCreateCardForm } =
    useContext(BoardContext)!;
  const unsent = usePending().waitingIds.has(id);

  return (
    <Card className="bg-card gap-0 py-0 overflow-hidden">
      <CardHeader
        className={`flex items-center justify-between px-4 py-3 [.border-b]:pb-3 bg-muted border-b cursor-grab`}
        {...listeners}
      >
        <CardTitle className="text-base font-bold text-heading w-full">
          {name}
          <span className="ml-2 font-normal text-muted-foreground">
            ({tasks.length})
          </span>
        </CardTitle>
        {unsent && <UnsentMark className="mr-1" />}
        <EditCardDialog
          id={id}
          name={name}
          description={description}
          tasks={tasks}
        />
      </CardHeader>
      <CardContent className={tasks.length ? "px-3 pt-3" : "px-3"}>
        {staticTasks ? (
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
  );
});

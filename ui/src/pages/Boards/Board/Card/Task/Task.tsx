import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { forwardRef, memo, useContext } from "react";
import EditTaskDialog from "./EditTaskDialog";
import type { SyntheticListenerMap } from "@dnd-kit/core/dist/hooks/utilities";
import { CheckCircle2, Circle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useParams } from "react-router";
import { BoardContext } from "../../BoardContext";
import { usePending } from "@/offline/pending";
import UnsentMark from "@/components/UnsentMark";

export type TaskType = {
  id: string;
  cardId: string;
  name: string;
  description: string;
  status: string;
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

    return (
      <Card
        ref={ref}
        {...rest}
        className="min-h-10 p-0 shadow-none rounded-md cursor-grab hover:border-link hover:bg-accent/40"
      >
        <TaskBody
          id={id}
          cardId={cardId}
          name={name}
          description={description}
          status={status}
          listeners={listeners}
        />
      </Card>
    );
  },
);

const TaskBody = memo(function TaskBody(
  props: TaskType & { listeners?: SyntheticListenerMap },
) {
  const { id, name, description, status, listeners, cardId } = props;
  const { boardId } = useParams();
  const { make } = useContext(BoardContext)!;
  const unsent = usePending().waitingIds.has(id);

  const toggleStatus = () => {
    void make({
      type: "updateTask",
      boardId: boardId!,
      cardId,
      taskId: id,
      status: status === "done" ? "pending" : "done",
    });
  };

  return (
    <CardHeader className="relative group flex items-center p-0 pr-2 h-full">
      <Button
        // Hidden until hovered, but still reachable by keyboard.
        className={`size-6 ml-2 transition-colors ${status === "done" ? "" : "opacity-0 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"}`}
        variant="ghost"
        size="icon"
        aria-label={status === "done" ? "Mark not done" : "Mark done"}
        onClick={toggleStatus}
      >
        {status === "done" ? (
          <CheckCircle2 className="size-4 text-success" />
        ) : (
          <Circle className="size-4 text-muted-foreground" />
        )}
      </Button>
      <CardTitle
        className={`h-full ml-1 flex-1 min-w-0 py-2 text-sm font-normal flex items-center ${status === "done" ? "text-muted-foreground line-through" : ""}`}
        {...listeners}
      >
        <span className="break-words">{name}</span>
      </CardTitle>
      {unsent && <UnsentMark className="mr-1" />}
      <EditTaskDialog
        id={id}
        cardId={cardId}
        name={name}
        description={description}
        status={status}
      />
    </CardHeader>
  );
});

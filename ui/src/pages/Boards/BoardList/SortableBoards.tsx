import { useNavigate } from "react-router";

import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  MouseSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  rectSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { Card, CardHeader, CardTitle } from "@/components/ui/card";

import type { Board } from "./BoardList";
import EditBoardDialog from "./EditBoardDialog";

type SortableBoardsProps = {
  boards: Board[];
  setBoards: React.Dispatch<React.SetStateAction<Board[]>>;
  editable?: boolean;
  children?: React.ReactNode;
};

export default function SortableBoards({
  boards,
  setBoards,
  editable = false,
  children,
}: SortableBoardsProps) {
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: { distance: 5 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const onDragEnd = async ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;

    const oldBoards = boards;
    const newBoards = arrayMove(
      boards,
      boards.findIndex((board) => board.id === active.id),
      boards.findIndex((board) => board.id === over.id),
    );
    setBoards(newBoards);

    const res = await fetch("/api/boards/order", {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ boardIds: newBoards.map((board) => board.id) }),
    });

    if (!res.ok) {
      setBoards(oldBoards);
    }
  };

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragEnd={onDragEnd}
    >
      <SortableContext items={boards} strategy={rectSortingStrategy}>
        <div className="flex flex-wrap items-start gap-8">
          {boards.map((board) => (
            <BoardTile
              key={board.id}
              board={board}
              setBoards={setBoards}
              editable={editable}
            />
          ))}
          {children}
        </div>
      </SortableContext>
    </DndContext>
  );
}

type BoardTileProps = {
  board: Board;
  setBoards: React.Dispatch<React.SetStateAction<Board[]>>;
  editable: boolean;
};

function BoardTile({ board, setBoards, editable }: BoardTileProps) {
  const navigate = useNavigate();

  const {
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
    setNodeRef,
  } = useSortable({
    id: board.id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    zIndex: isDragging ? 10 : undefined,
  };

  return (
    <Card
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      className={`group relative w-full max-w-xs aspect-video cursor-pointer p-4 gap-2 border-t-4 hover:bg-accent/40 ${editable ? "border-t-primary" : "border-t-link"}`}
      onClick={() => navigate(`/boards/${board.id}`)}
    >
      <CardHeader className="flex justify-between items-center p-0">
        <CardTitle className="text-link group-hover:underline">
          {board.name}
        </CardTitle>
        {editable && <EditBoardDialog {...board} setBoards={setBoards} />}
      </CardHeader>
    </Card>
  );
}

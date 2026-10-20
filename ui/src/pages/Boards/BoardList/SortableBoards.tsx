import { useNavigate } from "react-router";

import {
  closestCenter,
  DndContext,
  MouseSensor,
  TouchSensor,
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
import UnsentMark from "@/components/UnsentMark";
import { submit, useSync } from "@/offline/sync";
import { ago } from "@/offline/time";
import { OwnKeyboardSensor } from "@/utils/keyboardSensor";

import type { Board } from "./BoardList";
import EditBoardDialog from "./EditBoardDialog";
import FavoriteButton from "../FavoriteButton";
import { useMe } from "../favorites";
import type { BoardListData } from "../loaders";

type SortableBoardsProps = {
  boards: Board[];
  setBoards: React.Dispatch<React.SetStateAction<Board[]>>;
  offline?: BoardListData["offline"];
  editable?: boolean;
  children?: React.ReactNode;
};

export default function SortableBoards({
  boards,
  setBoards,
  offline,
  editable = false,
  children,
}: SortableBoardsProps) {
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: { distance: 5 },
    }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
    useSensor(OwnKeyboardSensor, {
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

    const saved = await submit({
      type: "orderBoards",
      boardIds: newBoards.map((board) => board.id),
    });
    if (!saved) {
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
              // Offline, only a Solo board can still be changed. A board
              // made offline isn't in the snapshot yet, but it's Solo.
              editable={editable && offline?.solo[board.id] !== false}
              savedAt={offline?.savedAt[board.id]}
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
  savedAt?: number;
};

function BoardTile({ board, setBoards, editable, savedAt }: BoardTileProps) {
  const navigate = useNavigate();
  const me = useMe();
  const unsent = useSync().pendingIds.has(board.id);

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
      className={`group relative w-full max-w-xs aspect-video cursor-pointer p-4 gap-2 border-t-4 hover:bg-accent/40 ${board.owner === me.id ? "border-t-primary" : "border-t-link"}`}
      onClick={() => navigate(`/boards/${board.id}`)}
    >
      <CardHeader className="flex justify-between items-center p-0">
        <CardTitle className="text-link group-hover:underline">
          {board.name}
        </CardTitle>
        <div className="flex items-center">
          {unsent && <UnsentMark />}
          <FavoriteButton board={board} />
          {editable && <EditBoardDialog {...board} setBoards={setBoards} />}
        </div>
      </CardHeader>
      {board.description && (
        <p className="text-sm text-muted-foreground line-clamp-2 break-words">
          {board.description}
        </p>
      )}
      <p className="mt-auto flex justify-between gap-2 text-xs text-muted-foreground tabular-nums">
        {savedAt !== undefined && <span>Updated {ago(savedAt)}</span>}
        <span className="ml-auto">
          {board.doneCount} / {board.taskCount} done
        </span>
      </p>
    </Card>
  );
}

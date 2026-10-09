import { useLoaderData, useParams } from "react-router";
import { Columns3, LayoutDashboard, Plus } from "lucide-react";

import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
} from "@dnd-kit/core";
import { KeyboardSensor, useSensor, useSensors } from "@dnd-kit/core";
import {
  horizontalListSortingStrategy,
  rectSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
} from "@dnd-kit/sortable";

import { Button } from "@/components/ui/button";

import useDnd from "@/hooks/useDnd";
import useSocket from "@/hooks/useSocket";
import { useBoardLayout, useColumnCount } from "@/hooks/useBoardLayout";

import Card, { DummyCard, type CardType } from "./Card/Card";
import { DummyTask, type TaskType } from "./Card/Task/Task";
import { useState } from "react";
import CreateCardForm from "./Card/CreateCardForm";
import type { boardViewLoader } from "../loaders";
import { BoardContext } from "./BoardContext";
import FavoriteButton from "../FavoriteButton";
import CleanUpButton from "./CleanUpButton";

export default function Board() {
  const [cards, setCards] = useState(
    useLoaderData<typeof boardViewLoader>().cardsWithTasks,
  );
  const { name, description, owner } = useLoaderData<typeof boardViewLoader>();
  const { boardId } = useParams();
  const [layout, setLayout] = useBoardLayout();
  const [listElement, setListElement] = useState<HTMLDivElement | null>(null);
  const columnCount = useColumnCount(layout === "masonry" ? listElement : null);

  const [activeTask, setActiveTask] = useState<TaskType | null>(null);
  const [activeCard, setActiveCard] = useState<CardType | null>(null);
  const [showCreateCardForm, setShowCreateCardForm] = useState(false);
  const [createTaskFormId, setCreateTaskFormId] = useState<string | null>(null);

  const boardContextValue = {
    cards,
    setCards,
    showCreateCardForm,
    setShowCreateCardForm,
    createTaskFormId,
    setCreateTaskFormId,
  };

  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: { distance: 5 },
    }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const {
    collisionDetection,
    onDragStart,
    onDragOver,
    onDragEnd,
    onDragCancel,
  } = useDnd(
    cards,
    setCards,
    activeCard,
    setActiveCard,
    activeTask,
    setActiveTask,
  );

  useSocket(boardId!, setCards);

  const createCard = (
    <div key="create-card" className="flex-shrink-0 w-80">
      {showCreateCardForm ? (
        <CreateCardForm />
      ) : (
        <Button
          variant="ghost"
          className="w-80 h-12 rounded-lg border-2 border-dashed border-border bg-card/60 hover:border-link hover:bg-accent text-muted-foreground hover:text-link"
          onClick={() => {
            setShowCreateCardForm(true);
            setCreateTaskFormId(null);
          }}
        >
          <Plus className="h-5 w-5" />
          Create a card
        </Button>
      )}
    </div>
  );

  const items = [
    ...cards.map((card) => <Card key={card.id} {...card} />),
    createCard,
  ];
  const columns = Array.from({ length: columnCount }, (_, column) =>
    items.filter((_, i) => i % columnCount === column),
  );

  const masonry = layout === "masonry";
  const toggleLabel = masonry
    ? "Show cards in one scrolling row"
    : "Show cards in columns";

  return (
    <BoardContext.Provider value={boardContextValue}>
      <div className="flex-1 min-w-0 px-8 pt-6">
        <div className="flex flex-col h-full">
          <header className="mb-6">
            <div className="flex items-center gap-1 mb-1">
              <h1 className="text-2xl font-bold">{name}</h1>
              <FavoriteButton board={{ id: boardId!, name, owner }} />
              <div className="ml-auto flex items-center gap-2">
                <Button
                  variant="outline"
                  size="icon"
                  aria-label={toggleLabel}
                  title={toggleLabel}
                  onClick={() => setLayout(masonry ? "scroll" : "masonry")}
                >
                  {masonry ? <Columns3 /> : <LayoutDashboard />}
                </Button>
                <CleanUpButton
                  boardId={boardId!}
                  cards={cards}
                  setCards={setCards}
                />
              </div>
            </div>
            {description && (
              <p className="text-sm text-muted-foreground max-w-prose break-words">
                {description}
              </p>
            )}
          </header>

          <DndContext
            sensors={sensors}
            collisionDetection={collisionDetection}
            onDragStart={onDragStart}
            onDragOver={onDragOver}
            onDragEnd={onDragEnd}
            onDragCancel={onDragCancel}
          >
            <div
              ref={setListElement}
              className={`flex gap-4 pb-6 flex-1 min-h-0 items-start -mx-8 px-8 ${masonry ? "overflow-y-auto" : "overflow-auto"}`}
            >
              <SortableContext
                items={cards}
                strategy={
                  masonry ? rectSortingStrategy : horizontalListSortingStrategy
                }
              >
                {masonry
                  ? columns.map((column, i) => (
                      <div key={i} className="flex flex-col gap-4">
                        {column}
                      </div>
                    ))
                  : items}

                <DragOverlay>
                  {activeTask ? (
                    <DummyTask {...activeTask} />
                  ) : activeCard ? (
                    <DummyCard {...activeCard} overlay />
                  ) : null}
                </DragOverlay>
              </SortableContext>
            </div>
          </DndContext>
        </div>
      </div>
    </BoardContext.Provider>
  );
}

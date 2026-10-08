import { useLoaderData, useParams } from "react-router";
import { Plus } from "lucide-react";

import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
} from "@dnd-kit/core";
import { KeyboardSensor, useSensor, useSensors } from "@dnd-kit/core";
import {
  horizontalListSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
} from "@dnd-kit/sortable";

import { Button } from "@/components/ui/button";

import useDnd from "@/hooks/useDnd";
import useSocket from "@/hooks/useSocket";

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
  const { name } = useLoaderData<typeof boardViewLoader>();
  const { boardId } = useParams();

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

  return (
    <BoardContext.Provider value={boardContextValue}>
      <div className="flex-1 min-w-0 px-8 pt-6">
        <div className="flex flex-col h-full">
          <header className="mb-6">
            <div className="flex items-center gap-1 mb-1">
              <h1 className="text-2xl font-bold">{name}</h1>
              <FavoriteButton board={{ id: boardId!, name }} />
              <div className="ml-auto">
                <CleanUpButton
                  boardId={boardId!}
                  cards={cards}
                  setCards={setCards}
                />
              </div>
            </div>
            <p className="text-sm text-muted-foreground">
              Organize your tasks with drag and drop
            </p>
          </header>

          <DndContext
            sensors={sensors}
            collisionDetection={collisionDetection}
            onDragStart={onDragStart}
            onDragOver={onDragOver}
            onDragEnd={onDragEnd}
            onDragCancel={onDragCancel}
          >
            <div className="flex gap-4 pb-6 flex-1 min-h-0 items-start overflow-auto">
              <SortableContext
                items={cards}
                strategy={horizontalListSortingStrategy}
              >
                {cards.map((card) => (
                  <Card key={card.id} {...card} />
                ))}

                <DragOverlay>
                  {activeTask ? (
                    <DummyTask {...activeTask} />
                  ) : activeCard ? (
                    <DummyCard {...activeCard} overlay />
                  ) : null}
                </DragOverlay>
              </SortableContext>

              <div className="flex-shrink-0 w-80">
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
            </div>
          </DndContext>
        </div>
      </div>
    </BoardContext.Provider>
  );
}

import { useLoaderData } from "react-router";
import { Plus } from "lucide-react";

import {
  closestCorners,
  DndContext,
  DragOverlay,
  MouseSensor,
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

import Card, { DummyCard, type CardType } from "./Card";
import { DummyTask, type TaskType } from "./Card/Task";
import { createContext, useState } from "react";
import CreateCardForm from "./Card/CreateCardForm";
import type { boardViewLoader } from "../BoardView";

type BoardContextType = {
  cards: CardType[];
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>;
  showCreateCardForm: boolean;
  setShowCreateCardForm: React.Dispatch<React.SetStateAction<boolean>>;
  createTaskFormId: string | null;
  setCreateTaskFormId: React.Dispatch<React.SetStateAction<string | null>>;
};

export const BoardContext = createContext<BoardContextType | null>(null);

export default function Board() {
  const [cards, setCards] = useState(
    useLoaderData<typeof boardViewLoader>().cardsWithTasks,
  );
  const { name } = useLoaderData<typeof boardViewLoader>();

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
    useSensor(MouseSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const { onDragStart, onDragOver, onDragEnd } = useDnd(
    cards,
    setCards,
    activeCard,
    setActiveCard,
    activeTask,
    setActiveTask,
  );

  useSocket(setCards);

  return (
    <BoardContext.Provider value={boardContextValue}>
      <div className="flex-1 p-6 ">
        <div className="flex flex-col h-full">
          <header className="mb-8">
            <h1 className="text-4xl font-bold text-primary mb-2">{name}</h1>
            <p className="text-muted-foreground">
              Organize your tasks with drag and drop
            </p>
          </header>

          <DndContext
            sensors={sensors}
            collisionDetection={closestCorners}
            onDragStart={onDragStart}
            onDragOver={onDragOver}
            onDragEnd={onDragEnd}
          >
            <div className="flex gap-6 pb-6 flex-1 items-start">
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
                    <DummyCard {...activeCard} />
                  ) : null}
                </DragOverlay>
              </SortableContext>

              <div className="flex-shrink-0 w-80">
                {showCreateCardForm ? (
                  <CreateCardForm />
                ) : (
                  <Button
                    variant="ghost"
                    className="w-80 h-12 border-2 border-dashed border-border hover:border-primary hover:bg-accent/50 text-muted-foreground hover:text-primary"
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

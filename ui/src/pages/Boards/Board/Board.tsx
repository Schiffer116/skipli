import { useLoaderData, useParams } from "react-router";
import { Columns3, LayoutDashboard, Plus } from "lucide-react";

import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
} from "@dnd-kit/core";
import { useSensor, useSensors } from "@dnd-kit/core";
import {
  horizontalListSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
} from "@dnd-kit/sortable";

import { Button } from "@/components/ui/button";

import useDnd from "@/hooks/useDnd";
import { useSwipeSnap } from "@/hooks/useSwipeSnap";
import { OwnKeyboardSensor } from "@/utils/keyboardSensor";
import useSocket from "@/hooks/useSocket";
import {
  CREATE_CARD_KEY,
  measureMasonryTarget,
  useBoardLayout,
  useMasonry,
} from "@/hooks/useBoardLayout";

import Card, { DummyCard, type CardType } from "./Card/Card";
import { DummyTask, type TaskType } from "./Card/Task/Task";
import { useState } from "react";
import CreateCardForm from "./Card/CreateCardForm";
import type { BoardViewData } from "../loaders";
import { BoardContext } from "./BoardContext";
import FavoriteButton from "../FavoriteButton";
import CleanUpButton from "./CleanUpButton";
import { useMe } from "../favorites";
import { useSync } from "@/offline/sync";
import { ago } from "@/offline/time";
import useSaveCards from "@/offline/useSaveCards";

export default function Board() {
  const loaderData = useLoaderData<BoardViewData>();
  const [cards, setCards] = useState(loaderData.cardsWithTasks);
  const { name, description, owner, members, savedAt } = loaderData;
  const { boardId } = useParams();
  const { offline } = useSync();
  // Offline, only a Solo board can be changed; a Shared board is read-only.
  const readOnly = offline && members.length > 1;
  useSaveCards(useMe().id, boardId!, cards);
  const [layout, setLayout] = useBoardLayout();
  const [listElement, setListElement] = useState<HTMLDivElement | null>(null);
  const [masonryElement, setMasonryElement] = useState<HTMLDivElement | null>(
    null,
  );

  const [activeTask, setActiveTask] = useState<TaskType | null>(null);
  const [activeCard, setActiveCard] = useState<CardType | null>(null);
  const [showCreateCardForm, setShowCreateCardForm] = useState(false);
  const [createTaskFormId, setCreateTaskFormId] = useState<string | null>(null);

  const boardContextValue = {
    readOnly,
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
    useSensor(OwnKeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const {
    collisionDetection,
    onDragStart,
    onDragOver,
    onDragMove,
    onDragEnd,
    onDragCancel,
  } = useDnd(
    cards,
    setCards,
    activeCard,
    setActiveCard,
    activeTask,
    setActiveTask,
    layout === "masonry",
  );

  useSocket(boardId!, setCards);

  const createCard = readOnly ? (
    <div key={CREATE_CARD_KEY} />
  ) : (
    <div key={CREATE_CARD_KEY} className="flex-shrink-0 w-80">
      {showCreateCardForm ? (
        <CreateCardForm />
      ) : (
        <Button
          variant="ghost"
          className="w-full h-12 rounded-lg border-2 border-dashed border-border bg-card/60 hover:border-link hover:bg-accent text-muted-foreground hover:text-link"
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
    ...cards.map((card) => (
      <Card key={card.id} {...card} staticTasks={activeCard !== null} />
    )),
    createCard,
  ];
  const masonry = layout === "masonry";
  const masonryLayout = useMasonry(masonry ? masonryElement : null, cards);
  useSwipeSnap(listElement, !masonry && !activeCard && !activeTask);
  const toggleLabel = masonry
    ? "Show cards in one scrolling row"
    : "Show cards in columns";

  return (
    <BoardContext.Provider value={boardContextValue}>
      <div className="flex-1 min-w-0 px-4 md:px-8 pt-6">
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
                {!readOnly && (
                  <CleanUpButton
                    boardId={boardId!}
                    cards={cards}
                    setCards={setCards}
                  />
                )}
              </div>
            </div>
            {description && (
              <p className="text-sm text-muted-foreground max-w-prose break-words">
                {description}
              </p>
            )}
            {/* Loaded online, the board was live until the connection
                dropped, so there's no "last updated" to show. */}
            {offline && (readOnly || savedAt !== undefined) && (
              <p className="mt-1 text-xs text-muted-foreground">
                {readOnly && "Read-only offline. "}
                {savedAt !== undefined && `Last updated ${ago(savedAt)}.`}
              </p>
            )}
          </header>

          <DndContext
            sensors={readOnly ? [] : sensors}
            collisionDetection={collisionDetection}
            measuring={{ droppable: { measure: measureMasonryTarget } }}
            onDragStart={onDragStart}
            onDragOver={onDragOver}
            onDragMove={onDragMove}
            onDragEnd={onDragEnd}
            onDragCancel={onDragCancel}
          >
            <div
              ref={setListElement}
              className={`flex gap-4 pb-6 flex-1 min-h-0 items-start -mx-4 px-4 md:-mx-8 md:px-8 ${masonry ? "overflow-y-auto [scrollbar-gutter:stable]" : "overflow-auto touch-pan-y max-md:*:w-full"}`}
            >
              <SortableContext
                items={cards}
                strategy={masonry ? () => null : horizontalListSortingStrategy}
              >
                {masonry ? (
                  <div
                    ref={setMasonryElement}
                    className="relative w-full shrink-0"
                    style={{ height: masonryLayout.height }}
                  >
                    {/* Cards mount once the width is known: measured at no
                        width their text wraps tall, and they would slide
                        to their real heights a frame later. Each stays
                        hidden until its own height is known. */}
                    {masonryLayout.count > 0 &&
                      items.map((item) => {
                        const key = String(item.key);
                        const { x, y, width } = masonryLayout.positions[key];
                        return (
                          <div
                            key={item.key}
                            ref={masonryLayout.measure}
                            data-key={item.key}
                            className={`absolute top-0 left-0 *:w-full ${masonryLayout.ready ? "transition-transform duration-225 ease-out" : ""} ${masonryLayout.measured(key) ? "" : "invisible"}`}
                            style={{
                              width,
                              transform: `translate(${x}px, ${y}px)`,
                            }}
                          >
                            {item}
                          </div>
                        );
                      })}
                  </div>
                ) : (
                  items
                )}

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

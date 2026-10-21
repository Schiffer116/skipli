import type { CardType } from "@/pages/Boards/Board/Card";
import type { TaskType } from "@/pages/Boards/Board/Card/Task";
import {
  closestCenter,
  getFirstCollision,
  pointerWithin,
  rectIntersection,
  type CollisionDetection,
  type DragEndEvent,
  type DragMoveEvent,
  type DragOverEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";
import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router";

import { apply } from "@/board/apply";
import type { BoardChange } from "@/offline/changes";

// Dragging shows the cards as they'd be if dropped there, a preview of
// this drag's own that nothing else sees or keeps. Only the drop makes a
// Board change; cancelling drops the preview.
export default function useDnd(
  cards: CardType[],
  make: (change: BoardChange) => Promise<void>,
  activeCard: CardType | null,
  setActiveCard: React.Dispatch<React.SetStateAction<CardType | null>>,
  activeTask: TaskType | null,
  setActiveTask: React.Dispatch<React.SetStateAction<TaskType | null>>,
  masonry: boolean,
) {
  const boardId = useParams().boardId!;
  const [preview, setPreview] = useState<CardType[] | null>(null);
  const shown = preview ?? cards;

  const findCard = (id: UniqueIdentifier, among = shown) => {
    const card = among.find((card) => card.id === id);
    if (card) return card;

    return among.find((card) => card.tasks.find((task) => task.id === id));
  };

  const onDragStart = (event: DragStartEvent) => {
    const { active } = event;

    const card = cards.find((card) => card.id === active.id);
    if (card) {
      setActiveCard(card);
      return;
    }

    const task = findCard(active.id, cards)?.tasks.find(
      (task) => task.id === active.id,
    );
    setActiveTask(task ?? null);
  };

  // Adapted from dnd-kit's multiple-containers example. With plain
  // closestCorners, moving a task into another card resizes both cards, the
  // dragged rect's nearest card flips back, the task moves back, and so on
  // forever ("Maximum update depth exceeded" from measureRects). Keying off
  // the pointer instead of the dragged rect makes the target stable.
  const lastOverId = useRef<UniqueIdentifier | null>(null);
  const recentlyMovedToNewCard = useRef(false);

  useEffect(() => {
    requestAnimationFrame(() => {
      recentlyMovedToNewCard.current = false;
    });
  }, [shown]);

  const collisionDetection: CollisionDetection = (args) => {
    // Dragging a card: only other cards are valid targets.
    if (activeCard) {
      const cardArgs = {
        ...args,
        droppableContainers: args.droppableContainers.filter((container) =>
          shown.some((card) => card.id === container.id),
        ),
      };
      if (masonry) {
        const pointerCards = pointerWithin(cardArgs);
        if (pointerCards.length > 0) return pointerCards;
      }
      return closestCenter(cardArgs);
    }

    const pointerIntersections = pointerWithin(args);
    const intersections =
      pointerIntersections.length > 0
        ? pointerIntersections
        : rectIntersection(args);
    let overId = getFirstCollision(intersections, "id");

    if (overId != null) {
      // Over a card: narrow down to the closest of its tasks, if any.
      const overCard = shown.find((card) => card.id === overId);
      if (overCard && overCard.tasks.length > 0) {
        overId =
          closestCenter({
            ...args,
            droppableContainers: args.droppableContainers.filter(
              (container) =>
                container.id !== overId &&
                overCard.tasks.some((task) => task.id === container.id),
            ),
          })[0]?.id ?? overId;
      }

      lastOverId.current = overId;
      return [{ id: overId }];
    }

    // Right after a move to a new card, layout hasn't settled; stay put.
    if (recentlyMovedToNewCard.current) {
      lastOverId.current = args.active.id;
    }
    return lastOverId.current ? [{ id: lastOverId.current }] : [];
  };

  // Only moving a task into a different card happens mid-drag: its new card
  // has to render it to make room. Reordering within one list (cards, or
  // tasks in the same card) is animated by SortableContext and committed in
  // onDragEnd; doing it here makes two items swap back and forth forever.
  const onDragOver = (event: DragOverEvent) => {
    const { active, over } = event;
    if (!over || activeCard) return;

    const currentCard = findCard(active.id);
    const overCard = findCard(over.id);
    if (!currentCard || !overCard || currentCard === overCard) return;

    const overTaskIndex = overCard.tasks.findIndex(
      (task) => task.id === over.id,
    );
    const translated = active.rect.current.translated;
    const isBelowOverTask =
      translated != null && translated.top > over.rect.top + over.rect.height;

    // Over the card itself rather than one of its tasks: append.
    const newTaskIndex =
      overTaskIndex === -1
        ? overCard.tasks.length
        : overTaskIndex + (isBelowOverTask ? 1 : 0);

    recentlyMovedToNewCard.current = true;

    setActiveTask(
      (task) =>
        task && {
          ...task,
          cardId: overCard.id,
        },
    );
    const tasks = overCard.tasks;
    setPreview((preview) =>
      apply(preview ?? cards, {
        type: "moveTask",
        boardId,
        cardId: currentCard.id,
        taskId: String(active.id),
        newCardId: overCard.id,
        beforeId: tasks[newTaskIndex - 1]?.id ?? null,
        afterId: tasks[newTaskIndex]?.id ?? null,
      }),
    );
  };

  // Masonry cards have different heights, so the layout reflows as the card
  // moves. Moving straight back to the slot it just left waits for the
  // pointer to travel, or two cards of different heights swap forever.
  const lastCardMove = useRef<{
    from: number;
    delta: DragMoveEvent["delta"];
  } | null>(null);

  const handledOverId = useRef<UniqueIdentifier | null>(null);

  const onDragMove = ({ active, over, delta }: DragMoveEvent) => {
    if (!masonry || !activeCard || !over || over.id === active.id) {
      handledOverId.current = null;
      return;
    }
    if (over.id === handledOverId.current) return;

    const from = shown.findIndex((card) => card.id === active.id);
    const to = shown.findIndex((card) => card.id === over.id);
    if (to === -1) return;

    const last = lastCardMove.current;
    if (
      last &&
      to === last.from &&
      Math.hypot(delta.x - last.delta.x, delta.y - last.delta.y) < 40
    ) {
      return;
    }

    handledOverId.current = over.id;
    lastCardMove.current = { from, delta };
    setPreview((preview) => arrayMove(preview ?? cards, from, to));
  };

  const endDrag = () => {
    lastCardMove.current = null;
    handledOverId.current = null;
    lastOverId.current = null;
    setPreview(null);
    setActiveTask(null);
    setActiveCard(null);
  };

  const onDragCancel = endDrag;

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    endDrag();
    const id = String(active.id);

    if (activeCard) {
      const overCard = over ? findCard(over.id) : undefined;
      const dropped =
        !masonry && overCard
          ? arrayMove(
              shown,
              shown.findIndex((card) => card.id === id),
              shown.findIndex((card) => card.id === overCard.id),
            )
          : shown;
      const index = dropped.findIndex((card) => card.id === id);
      const beforeId = dropped[index - 1]?.id ?? null;
      const afterId = dropped[index + 1]?.id ?? null;
      const from = cards.findIndex((card) => card.id === id);
      if (
        from === -1 ||
        (cards[from - 1]?.id === beforeId && cards[from + 1]?.id === afterId)
      ) {
        return;
      }

      void make({ type: "moveCard", boardId, cardId: id, beforeId, afterId });
      return;
    }

    if (!activeTask) return;

    // Where the task is now (onDragOver may already have moved it to
    // another card), then any final reorder within that card.
    const taskCard = findCard(id)!;
    let tasks = taskCard.tasks;
    if (over && over.id !== taskCard.id && findCard(over.id) === taskCard) {
      tasks = arrayMove(
        tasks,
        tasks.findIndex((task) => task.id === id),
        tasks.findIndex((task) => task.id === over.id),
      );
    }
    const index = tasks.findIndex((task) => task.id === id);
    const beforeId = tasks[index - 1]?.id ?? null;
    const afterId = tasks[index + 1]?.id ?? null;

    const fromCard = findCard(id, cards);
    if (!fromCard) return;
    const from = fromCard.tasks.findIndex((task) => task.id === id);
    if (
      fromCard.id === taskCard.id &&
      fromCard.tasks[from - 1]?.id === beforeId &&
      fromCard.tasks[from + 1]?.id === afterId
    ) {
      return;
    }

    void make({
      type: "moveTask",
      boardId,
      cardId: fromCard.id,
      taskId: id,
      newCardId: taskCard.id,
      beforeId,
      afterId,
    });
  };

  return {
    shown,
    collisionDetection,
    onDragStart,
    onDragOver,
    onDragMove,
    onDragEnd,
    onDragCancel,
  };
}

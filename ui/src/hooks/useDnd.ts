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
import { useEffect, useRef } from "react";
import { useParams } from "react-router";

import { socket } from "@/utils/socket";
import { moveCard, moveTask } from "@/utils/updateCard";
import { submit } from "@/offline/sync";

export default function useDnd(
  cards: CardType[],
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>,
  activeCard: CardType | null,
  setActiveCard: React.Dispatch<React.SetStateAction<CardType | null>>,
  activeTask: TaskType | null,
  setActiveTask: React.Dispatch<React.SetStateAction<TaskType | null>>,
  masonry: boolean,
) {
  const boardId = useParams().boardId!;
  const oldCards = useRef<CardType[]>(structuredClone(cards));
  const oldCard = useRef<CardType>(structuredClone(activeCard));

  const findCard = (id: UniqueIdentifier) => {
    const card = cards.find((card) => card.id === id);
    if (card) return card;

    return cards.find((card) => card.tasks.find((task) => task.id === id));
  };

  const onDragStart = (event: DragStartEvent) => {
    const { active } = event;

    oldCards.current = structuredClone(cards);
    oldCard.current = cards.find((card) => card.id === active.id) ?? null;
    if (oldCard.current) {
      setActiveCard(oldCard.current);
      return;
    }

    oldCard.current =
      cards.find((card) => card.tasks.find((task) => task.id === active.id)) ??
      null;
    const task = oldCard.current?.tasks.find((task) => task.id === active.id);
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
  }, [cards]);

  const collisionDetection: CollisionDetection = (args) => {
    // Dragging a card: only other cards are valid targets.
    if (activeCard) {
      const cardArgs = {
        ...args,
        droppableContainers: args.droppableContainers.filter((container) =>
          cards.some((card) => card.id === container.id),
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
      const overCard = cards.find((card) => card.id === overId);
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
    setCards(moveTask(active.id, overCard.id, newTaskIndex));
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

    const from = cards.findIndex((card) => card.id === active.id);
    const to = cards.findIndex((card) => card.id === over.id);
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
    setCards(moveCard(from, to));
  };

  const endDrag = () => {
    lastCardMove.current = null;
    handledOverId.current = null;
    lastOverId.current = null;
    setActiveTask(null);
    setActiveCard(null);
  };

  const onDragCancel = () => {
    setCards(oldCards.current);
    endDrag();
  };

  const onDragEnd = async ({ active, over }: DragEndEvent) => {
    if (activeCard) {
      endDrag();
      const oldCardIndex = oldCards.current.findIndex(
        (card) => card.id === active.id,
      );
      const overCard = over ? findCard(over.id) : undefined;
      const newCards =
        !masonry && overCard
          ? moveCard(
              cards.findIndex((card) => card.id === active.id),
              cards.findIndex((card) => card.id === overCard.id),
            )(cards)
          : cards;
      const newCardIndex = newCards.findIndex((card) => card.id === active.id);
      if (oldCardIndex === newCardIndex) return;

      setCards(newCards);

      const saved = await submit({
        type: "moveCard",
        boardId,
        cardId: String(active.id),
        beforeId: newCards[newCardIndex - 1]?.id ?? null,
        afterId: newCards[newCardIndex + 1]?.id ?? null,
      });
      if (!saved) {
        setCards(oldCards.current);
        return;
      }

      socket.emit("move card", oldCardIndex, newCardIndex);
      return;
    }

    if (!activeTask) return;
    endDrag();

    // Where the task is now (onDragOver may already have moved it to
    // another card), then apply any final reorder within that card.
    let newCards = cards;
    const taskCard = findCard(active.id)!;
    if (over && over.id !== taskCard.id && findCard(over.id) === taskCard) {
      const overTaskIndex = taskCard.tasks.findIndex(
        (task) => task.id === over.id,
      );
      newCards = moveTask(active.id, taskCard.id, overTaskIndex)(cards);
    }
    setCards(newCards);

    const tasks = newCards.find((card) => card.id === taskCard.id)!.tasks;
    const taskIndex = tasks.findIndex((task) => task.id === active.id);
    const originalCard = oldCard.current!;
    const originalIndex = originalCard.tasks.findIndex(
      (task) => task.id === active.id,
    );
    if (originalCard.id === taskCard.id && originalIndex === taskIndex) return;

    const saved = await submit({
      type: "moveTask",
      boardId,
      cardId: originalCard.id,
      taskId: String(active.id),
      newCardId: taskCard.id,
      beforeId: tasks[taskIndex - 1]?.id ?? null,
      afterId: tasks[taskIndex + 1]?.id ?? null,
    });
    if (!saved) {
      setCards(oldCards.current);
      return;
    }

    socket.emit("move task", active.id, taskCard.id, taskIndex);
  };

  return {
    collisionDetection,
    onDragStart,
    onDragOver,
    onDragMove,
    onDragEnd,
    onDragCancel,
  };
}

import type { CardType } from "@/pages/Boards/Board/Card";
import type { TaskType } from "@/pages/Boards/Board/Card/Task";
import type {
  DragOverEvent,
  DragStartEvent,
  UniqueIdentifier,
} from "@dnd-kit/core";
import { useRef } from "react";
import { useParams } from "react-router";

import { socket } from "@/main";
import { moveCard, moveTask } from "@/utils/updateCard";

export default function useDnd(
  cards: CardType[],
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>,
  activeCard: CardType | null,
  setActiveCard: React.Dispatch<React.SetStateAction<CardType | null>>,
  activeTask: TaskType | null,
  setActiveTask: React.Dispatch<React.SetStateAction<TaskType | null>>,
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
    const task = oldCard.current?.tasks.find((task) => task.id === active.id)!;
    setActiveTask(task);
  };

  const onDragOver = (event: DragOverEvent) => {
    const { active, over } = event;
    if (!over) return;

    const currentCard = findCard(active.id);
    const overCard = findCard(over.id);
    if (!currentCard || !overCard) {
      return;
    }

    if (cards.find((card) => card.id === active.id)) {
      const activeCardIndex = cards.findIndex((card) => card.id === active.id);
      const overCardIndex = cards.findIndex((card) => card.id === overCard.id);
      setCards(moveCard(activeCardIndex, overCardIndex));
      return;
    }

    if (currentCard === overCard) {
      const overTaskIndex = overCard.tasks.findIndex(
        (task) => task.id === over!.id,
      );
      setCards(moveTask(active.id, overCard.id, overTaskIndex));
      return;
    }

    const activeTaskTop = active.rect.current.translated?.top;
    const lastTaskBottom = over.rect.bottom;
    const isBelowLastTask = activeTaskTop! > lastTaskBottom;

    const overTaskIndex = isBelowLastTask
      ? overCard.tasks.length
      : overCard.tasks.findIndex((task) => task.id === over!.id);

    setActiveTask(
      (task) =>
        task && {
          ...task,
          cardId: overCard.id,
        },
    );
    setCards(moveTask(active.id, overCard.id, overTaskIndex));
  };

  const onDragEnd = async () => {
    if (activeCard) {
      const activeCardIndex = cards.findIndex(
        (card) => card.id === activeCard.id,
      );
      const beforeId = cards[activeCardIndex - 1]?.id ?? null;
      const afterId = cards[activeCardIndex + 1]?.id ?? null;

      const res = await fetch(`/api/boards/${boardId}/cards/${activeCard.id}`, {
        method: "PATCH",
        headers: {
          Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          beforeId,
          afterId,
        }),
      });

      if (!res.ok) {
        setCards(oldCards.current!);
        return;
      }

      const oldCardIndex = oldCards.current!.findIndex(
        (card) => card.id === oldCard.current!.id,
      );
      socket.emit("move card", oldCardIndex, activeCardIndex);
    }

    if (activeTask) {
      const activeTaskCard = cards.find(
        (card) => card.id === activeTask.cardId,
      )!;
      const activeTaskIndex = activeTaskCard.tasks.findIndex(
        (task) => task.id === activeTask.id,
      );

      const beforeId = activeTaskCard!.tasks[activeTaskIndex - 1]?.id ?? null;
      const afterId = activeTaskCard!.tasks[activeTaskIndex + 1]?.id ?? null;

      const res = await fetch(
        `/api/boards/${boardId}/cards/${oldCard.current!.id}/tasks/${activeTask.id}`,
        {
          method: "PATCH",
          headers: {
            Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            newCardId: activeTask.cardId,
            beforeId,
            afterId,
          }),
        },
      );

      if (!res.ok) {
        setCards(oldCards.current!);
        return;
      }

      socket.emit(
        "move task",
        activeTask.id,
        activeTaskCard.id,
        activeTaskIndex,
      );
    }

    setActiveTask(null);
    setActiveCard(null);
  };

  return {
    onDragStart,
    onDragOver,
    onDragEnd,
  };
}

import type { CardType } from "@/pages/Boards/Board/Card";
import type { TaskType } from "@/pages/Boards/Board/Card/Task";
import type { UniqueIdentifier } from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";

export const createCard = (card: CardType) => (cards: CardType[]) => [
  ...cards,
  card,
];

export const updateCard =
  (updatedCard: Omit<CardType, "tasks">) => (cards: CardType[]) =>
    cards.map((card) =>
      card.id === updatedCard.id ? { ...card, ...updatedCard } : card,
    );

export const deleteCard = (cardId: string) => (cards: CardType[]) =>
  cards.filter((card) => card.id !== cardId);

export const moveCard =
  (activeCardIndex: number, overCardIndex: number) => (cards: CardType[]) =>
    arrayMove(cards, activeCardIndex, overCardIndex);

export const createTask = (task: TaskType) => (cards: CardType[]) =>
  cards.map((card) =>
    card.id === task.cardId ? { ...card, tasks: [...card.tasks, task] } : card,
  );

export const updateTask = (updatedTask: TaskType) => (cards: CardType[]) =>
  cards.map((card) => ({
    ...card,
    tasks: card.tasks.map((task) =>
      task.id === updatedTask.id ? updatedTask : task,
    ),
  }));

export const deleteTask = (id: string) => (cards: CardType[]) =>
  cards.map((card) => ({
    ...card,
    tasks: card.tasks.filter((task) => task.id !== id),
  }));

export const moveTask =
  (
    taskId: UniqueIdentifier,
    newCardId: UniqueIdentifier,
    newTaskIndex: number,
  ) =>
    (cards: CardType[]) => {
      const oldCard = cards.find((card) =>
        card.tasks.find((task) => task.id == taskId),
      )!;
      const newCard = cards.find((card) => card.id === newCardId)!;

      if (newCard.id === oldCard.id) {
        return cards.map((card) =>
          card.id === newCardId
            ? {
              ...card,
              tasks: arrayMove(
                card.tasks,
                oldCard.tasks.findIndex((task) => task.id === taskId),
                newTaskIndex,
              ),
            }
            : card,
        );
      }

      const [task] = oldCard.tasks.splice(
        oldCard.tasks.findIndex((task) => task.id === taskId),
        1,
      );
      newCard.tasks.splice(newTaskIndex, 0, {
        ...task,
        cardId: newCardId as string,
      });

      return [...cards];
    };

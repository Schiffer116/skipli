import { useEffect } from "react";

import { socket } from "@/main";
import type { CardType } from "@/pages/Boards/Board/Card";
import type { TaskType } from "@/pages/Boards/Board/Card/Task";

import {
  createCard,
  deleteCard,
  moveCard,
  updateCard,
  createTask,
  updateTask,
  deleteTask,
  moveTask,
} from "@/utils/updateCard";
import type { UniqueIdentifier } from "@dnd-kit/core";

export default function useSocket(
  setCards: React.Dispatch<React.SetStateAction<CardType[]>>,
) {
  useEffect(() => {
    function onCreateCard(card: CardType) {
      setCards(createCard(card));
    }

    function onUpdateCard(updatedCard: Omit<CardType, "tasks">) {
      setCards(updateCard(updatedCard));
    }

    function onDeleteCard(cardId: string) {
      setCards(deleteCard(cardId));
    }

    function onMoveCard(activeCardIndex: number, overCardIndex: number) {
      setCards(moveCard(activeCardIndex, overCardIndex));
    }

    function onCreateTask(task: TaskType) {
      setCards(createTask(task));
    }

    function onUpdateTask(updatedTask: TaskType) {
      setCards(updateTask(updatedTask));
    }

    function onDeleteTask(id: string) {
      setCards(deleteTask(id));
    }

    function onMoveTask(
      taskId: UniqueIdentifier,
      newCardId: UniqueIdentifier,
      newTaskIndex: number,
    ) {
      setCards(moveTask(taskId, newCardId, newTaskIndex));
    }

    const events: [string, (...args: any[]) => void][] = [
      ["create card", onCreateCard],
      ["update card", onUpdateCard],
      ["delete card", onDeleteCard],
      ["move card", onMoveCard],
      ["create task", onCreateTask],
      ["update task", onUpdateTask],
      ["delete task", onDeleteTask],
      ["move task", onMoveTask],
    ];

    events.forEach(([event, handler]) => {
      socket.on(event, handler);
    });

    return () => {
      events.forEach(([event, handler]) => {
        socket.off(event, handler);
      });
    };
  }, []);
}

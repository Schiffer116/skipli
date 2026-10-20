import type { BoardChange } from "@/offline/changes";
import type { CardType } from "@/pages/Boards/Board/Card";
import type { TaskType } from "@/pages/Boards/Board/Card/Task";

// Shows a Board change on a board's cards: one made here, or one a
// teammate made. Applying a change twice, or to cards that never had what it
// changes (created twice, or edited after a delete), changes nothing more.
export function apply(cards: CardType[], change: BoardChange): CardType[] {
  switch (change.type) {
    case "createCard":
      if (cards.some((card) => card.id === change.cardId)) return cards;
      return [
        ...cards,
        { id: change.cardId, name: change.name, description: "", tasks: [] },
      ];

    case "updateCard":
      return cards.map((card) =>
        card.id === change.cardId
          ? { ...card, name: change.name, description: change.description }
          : card,
      );

    case "deleteCard":
      return cards.filter((card) => card.id !== change.cardId);

    case "moveCard": {
      const card = cards.find((card) => card.id === change.cardId);
      if (!card) return cards;
      return placed(
        cards.filter((other) => other !== card),
        card,
        change.beforeId,
        change.afterId,
      );
    }

    case "createTask": {
      if (findTask(cards, change.taskId)) return cards;
      const task: TaskType = {
        id: change.taskId,
        cardId: change.cardId,
        name: change.name,
        description: "",
        status: "pending",
      };
      return cards.map((card) =>
        card.id === change.cardId
          ? { ...card, tasks: [...card.tasks, task] }
          : card,
      );
    }

    case "updateTask":
      return cards.map((card) =>
        card.tasks.some((task) => task.id === change.taskId)
          ? {
              ...card,
              tasks: card.tasks.map((task) =>
                task.id === change.taskId
                  ? {
                      ...task,
                      name: change.name,
                      description: change.description,
                      status: change.status,
                    }
                  : task,
              ),
            }
          : card,
      );

    case "deleteTask":
      return withoutTask(cards, change.taskId);

    case "moveTask": {
      const task = findTask(cards, change.taskId);
      if (!task || !cards.some((card) => card.id === change.newCardId)) {
        return cards;
      }
      const moved = { ...task, cardId: change.newCardId };
      return withoutTask(cards, change.taskId).map((card) =>
        card.id === change.newCardId
          ? {
              ...card,
              tasks: placed(card.tasks, moved, change.beforeId, change.afterId),
            }
          : card,
      );
    }

    case "cleanUp":
      return cards.map((card) =>
        card.tasks.some((task) => task.status === "done")
          ? { ...card, tasks: card.tasks.filter((t) => t.status !== "done") }
          : card,
      );
  }
}

// Puts an item right after the one before it, else right before the one
// after it, else at the end.
function placed<T extends { id: string }>(
  items: T[],
  item: T,
  beforeId: string | null,
  afterId: string | null,
): T[] {
  const before = items.findIndex((other) => other.id === beforeId);
  const after = items.findIndex((other) => other.id === afterId);
  const at = before !== -1 ? before + 1 : after !== -1 ? after : items.length;
  return [...items.slice(0, at), item, ...items.slice(at)];
}

function findTask(cards: CardType[], taskId: string) {
  for (const card of cards) {
    const task = card.tasks.find((task) => task.id === taskId);
    if (task) return task;
  }
}

function withoutTask(cards: CardType[], taskId: string) {
  return cards.map((card) =>
    card.tasks.some((task) => task.id === taskId)
      ? { ...card, tasks: card.tasks.filter((task) => task.id !== taskId) }
      : card,
  );
}

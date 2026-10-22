import { fields, type BoardChange } from "@/offline/changes";
import type { CardType } from "@/pages/Boards/Board/Card";
import type { TaskType } from "@/pages/Boards/Board/Card/Task";

// Shows a Board change on a board's cards: one made here, or one a
// teammate made. Applying a change twice, or to cards that never had what it
// changes (created twice, or edited after a delete), changes nothing more.
export function apply(cards: CardType[], change: BoardChange): CardType[] {
  const next = applyOrDrop(cards, change);
  return next === "dropped" ? cards : next;
}

// The cards after a change, or "dropped" when the server refuses it and it
// would be dropped: a change to a card or task a teammate deleted. The
// server gives the same result for every change (both pass
// api/internal/boardchange/testdata/board-changes.json).
export function applyOrDrop(
  cards: CardType[],
  change: BoardChange,
): CardType[] | "dropped" {
  switch (change.type) {
    case "createCard":
      if (cards.some((card) => card.id === change.cardId)) return cards;
      return [
        ...cards,
        { id: change.cardId, name: change.name, description: "", tasks: [] },
      ];

    case "updateCard": {
      const changed = fields(change);
      if (!cards.some((card) => card.id === change.cardId)) return "dropped";
      if (Object.keys(changed).length === 0) return "dropped";
      return cards.map((card) =>
        card.id === change.cardId ? { ...card, ...changed } : card,
      );
    }

    case "deleteCard":
      return cards.filter((card) => card.id !== change.cardId);

    case "moveCard": {
      const card = cards.find((card) => card.id === change.cardId);
      if (!card) return "dropped";
      return placed(
        cards.filter((other) => other !== card),
        card,
        change.beforeId,
        change.afterId,
      );
    }

    case "createTask": {
      // Wherever it is now: a teammate may have moved it since.
      if (findTask(cards, change.taskId)) return cards;
      if (!cards.some((card) => card.id === change.cardId)) return "dropped";
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

    case "updateTask": {
      const changed = fields(change);
      if (!findTask(cards, change.taskId)) return "dropped";
      if (Object.keys(changed).length === 0) return "dropped";
      return cards.map((card) =>
        card.tasks.some((task) => task.id === change.taskId)
          ? {
              ...card,
              tasks: card.tasks.map((task) =>
                task.id === change.taskId ? { ...task, ...changed } : task,
              ),
            }
          : card,
      );
    }

    case "deleteTask":
      return withoutTasks(cards, [change.taskId]);

    case "moveTask": {
      const task = findTask(cards, change.taskId);
      if (!task || !cards.some((card) => card.id === change.newCardId)) {
        return "dropped";
      }
      const moved = { ...task, cardId: change.newCardId };
      return withoutTasks(cards, [change.taskId]).map((card) =>
        card.id === change.newCardId
          ? {
              ...card,
              tasks: placed(card.tasks, moved, change.beforeId, change.afterId),
            }
          : card,
      );
    }

    case "cleanUp":
      return withoutTasks(cards, change.taskIds);
  }
}

// Puts an item right after the one before it, else right before the one
// after it, else at the end: a neighbour a teammate moved away or deleted
// never stops a move.
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

function withoutTasks(cards: CardType[], taskIds: string[]) {
  const gone = (task: TaskType) => taskIds.includes(task.id);
  return cards.map((card) =>
    card.tasks.some(gone)
      ? { ...card, tasks: card.tasks.filter((task) => !gone(task)) }
      : card,
  );
}

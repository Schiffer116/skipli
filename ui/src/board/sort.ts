import type { BoardChange } from "@/offline/changes";
import { apiFetch } from "@/utils/apiFetch";

export type Sorted = {
  // The card's task ids as the criterion puts them.
  order: string[];
  // The Jev release that judged, "fake" on a developer's machine.
  model: string;
};

// Why a sort failed, in a line for the dialog.
export class SortError extends Error {}

// Asks the server what order a criterion puts a card's tasks in. Nothing
// changes on the server: the app makes the moves (sortMoves).
export async function sortCard(
  boardId: string,
  cardId: string,
  criterion: string,
): Promise<Sorted> {
  let res: Response;
  try {
    res = await apiFetch(
      `/api/boards/${encodeURIComponent(boardId)}/cards/${encodeURIComponent(cardId)}/sort`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ criterion }),
      },
    );
  } catch {
    throw new SortError("Couldn't reach the server. Try again.");
  }
  if (res.ok) return res.json();
  switch (res.status) {
    case 413:
      throw new SortError("This card has too many tasks to sort.");
    case 429: {
      const wait = Number(res.headers.get("Retry-After"));
      throw new SortError(
        wait > 60
          ? "You've sorted a lot this hour. Try again later."
          : "Sorting too often. Try again in a few seconds.",
      );
    }
    case 503:
      throw new SortError(
        (await res.text()).startsWith("sorting is not configured")
          ? "Sorting isn't set up on this server."
          : "Jev isn't answering. Try again.",
      );
    default:
      throw new SortError("Couldn't sort this card. Try again.");
  }
}

// The fewest moveTask changes that put a card's tasks (current, top to
// bottom) in target's order. The tasks already in order relative to each
// other (the longest run of them) stay put, and each other one goes right
// below the task the target puts above it. Ids in only one of the two
// lists, tasks added or deleted while the order was being worked out, are
// left alone.
export function sortMoves(
  boardId: string,
  cardId: string,
  current: string[],
  target: string[],
): BoardChange[] {
  const present = new Set(current);
  const wanted = target.filter((id) => present.has(id));
  const rank = new Map(wanted.map((id, i) => [id, i]));
  const kept = longestIncreasing(
    current.filter((id) => rank.has(id)).map((id) => rank.get(id)!),
  );

  const order = [...current];
  const moves: BoardChange[] = [];
  wanted.forEach((id, i) => {
    if (kept.has(i)) return;
    order.splice(order.indexOf(id), 1);
    // Below the task before it, or else at the top.
    const at = i === 0 ? 0 : order.indexOf(wanted[i - 1]) + 1;
    order.splice(at, 0, id);
    moves.push({
      type: "moveTask",
      boardId,
      cardId,
      taskId: id,
      newCardId: cardId,
      beforeId: order[at - 1] ?? null,
      afterId: order[at + 1] ?? null,
    });
  });
  return moves;
}

// The values of one longest strictly increasing subsequence of values.
function longestIncreasing(values: number[]): Set<number> {
  const length = values.map(() => 1);
  const previous = values.map(() => -1);
  let end = -1;
  values.forEach((value, i) => {
    for (let j = 0; j < i; j++) {
      if (values[j] < value && length[j] + 1 > length[i]) {
        length[i] = length[j] + 1;
        previous[i] = j;
      }
    }
    if (end === -1 || length[i] > length[end]) end = i;
  });
  const kept = new Set<number>();
  for (let i = end; i !== -1; i = previous[i]) kept.add(values[i]);
  return kept;
}

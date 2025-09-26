import type { LoaderFunctionArgs } from "react-router";

import Board from "./Board/Board";
import { fetchCards, type CardType } from "./Board/Card";
import { fetchTasks } from "./Board/Card/Task/Task";
import Sidebar from "./Board/Sidebar";

export async function boardViewLoader({ params }: LoaderFunctionArgs) {
  const boardId = params.boardId!;

  const cards = await fetchCards(boardId);
  const cardsWithTasks = await Promise.all(
    cards.map(async (card: CardType) => {
      const tasks = await fetchTasks(boardId, card.id);
      return { ...card, tasks };
    }),
  );

  const { name } = await fetch(`/api/boards/${boardId}`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  }).then((res) => res.json());

  const res = await fetch(`/api/boards/${boardId}/members`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  const members: string[] = await res.json();

  return { cardsWithTasks, name, members };
}

export default function BoardView() {
  return (
    <div className="flex-1 flex w-fit">
      <Sidebar />
      <Board />
    </div>
  );
}

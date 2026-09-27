// Route loaders for /boards, kept out of the route component files so those
// only export components and stay hot-reloadable.
import { redirect, type LoaderFunctionArgs } from "react-router";

import { throwIfNotOk } from "@/utils/throwIfNotOk";

import type { Board } from "./BoardList/BoardList";
import type { CardType } from "./Board/Card";
import type { TaskType } from "./Board/Card/Task";

export async function boardsLoader() {
  const email = await fetch("/api/auth/email", {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  if (email.status === 401) {
    return redirect("/login");
  }

  return await email.json();
}

export const boardListLoader = async (): Promise<{
  boards: Board[];
  teamBoards: Board[];
}> => {
  const res = await fetch("/api/boards", {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });

  if (!res.ok) {
    throw redirect("/login");
  }
  const { boards, teamBoards } = await res.json();
  const boardIds = boards.map((board: Board) => board.id);

  console.log("board", boards);

  return {
    boards,
    teamBoards: teamBoards.filter(
      (board: Board) => !boardIds.includes(board.id),
    ),
  };
};

async function fetchCards(boardId: string) {
  const data = await fetch(`/api/boards/${boardId}/cards`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });
  throwIfNotOk(data);
  const cards: CardType[] = await data.json();
  return cards;
}

async function fetchTasks(boardId: string, cardId: string) {
  const data = await fetch(`/api/boards/${boardId}/cards/${cardId}/tasks`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });
  throwIfNotOk(data);
  const tasks: TaskType[] = await data.json();
  return tasks;
}

export async function boardViewLoader({ params }: LoaderFunctionArgs) {
  const boardId = params.boardId!;

  // Board first, so a bad id is a clean 404 rather than a failed card fetch.
  const boardRes = await fetch(`/api/boards/${boardId}`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });
  throwIfNotOk(boardRes);
  const { name } = await boardRes.json();

  const cards = await fetchCards(boardId);
  const cardsWithTasks = await Promise.all(
    cards.map(async (card: CardType) => {
      const tasks = await fetchTasks(boardId, card.id);
      return { ...card, tasks };
    }),
  );

  const res = await fetch(`/api/boards/${boardId}/members`, {
    headers: {
      Authorization: `Bearer ${localStorage.getItem("accessToken")}`,
    },
  });
  throwIfNotOk(res);

  const members: string[] = await res.json();

  return { cardsWithTasks, name, members };
}

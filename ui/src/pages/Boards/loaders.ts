// Route loaders for /boards, kept out of the route component files so those
// only export components and stay hot-reloadable.
import { redirect, type LoaderFunctionArgs } from "react-router";

import { throwIfNotOk } from "@/utils/throwIfNotOk";

import type { Board } from "./BoardList/BoardList";
import type { CardType } from "./Board/Card";
import type { TaskType } from "./Board/Card/Task";
import { apiFetch } from "@/utils/apiFetch";

export type Favorite = Pick<Board, "id" | "name">;

export async function boardsLoader() {
  const [emailRes, boardsRes] = await Promise.all([
    apiFetch("/api/auth/email"),
    apiFetch("/api/boards"),
  ]);

  if (emailRes.status === 401 || boardsRes.status === 401) {
    throw redirect("/login");
  }

  const { email }: { email: string } = await emailRes.json();
  const { boards, teamBoards }: { boards: Board[]; teamBoards: Board[] } =
    await boardsRes.json();
  const favorites: Favorite[] = [...boards, ...teamBoards]
    .filter((board) => board.favorite)
    .map(({ id, name }) => ({ id, name }));

  return { email, favorites };
}

export const boardListLoader = async (): Promise<{
  boards: Board[];
  teamBoards: Board[];
}> => {
  const res = await apiFetch("/api/boards");

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
  const data = await apiFetch(`/api/boards/${boardId}/cards`);
  throwIfNotOk(data);
  const cards: CardType[] = await data.json();
  return cards;
}

async function fetchTasks(boardId: string, cardId: string) {
  const data = await apiFetch(`/api/boards/${boardId}/cards/${cardId}/tasks`);
  throwIfNotOk(data);
  const tasks: TaskType[] = await data.json();
  return tasks;
}

export async function boardViewLoader({ params }: LoaderFunctionArgs) {
  const boardId = params.boardId!;

  // Board first, so a bad id is a clean 404 rather than a failed card fetch.
  const boardRes = await apiFetch(`/api/boards/${boardId}`);
  throwIfNotOk(boardRes);
  const { name, description } = await boardRes.json();

  const cards = await fetchCards(boardId);
  const cardsWithTasks = await Promise.all(
    cards.map(async (card: CardType) => {
      const tasks = await fetchTasks(boardId, card.id);
      return { ...card, tasks };
    }),
  );

  const res = await apiFetch(`/api/boards/${boardId}/members`);
  throwIfNotOk(res);

  const members: string[] = await res.json();

  return { cardsWithTasks, name, description, members };
}

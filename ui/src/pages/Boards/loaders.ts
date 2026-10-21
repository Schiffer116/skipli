// Route loaders for /boards, kept out of the route component files so those
// only export components and stay hot-reloadable.
//
// Online they load from the server and keep what they load in the Offline
// copy; offline (the browser says so) they read the Offline copy instead.
import {
  data,
  redirect,
  type LoaderFunctionArgs,
  type ShouldRevalidateFunction,
} from "react-router";

import { throwIfNotOk } from "@/utils/throwIfNotOk";

import type { Board } from "./BoardList/BoardList";
import type { CardType } from "./Board/Card";
import { apiFetch } from "@/utils/apiFetch";
import { fetchBoard, type FetchedBoard } from "@/board/fetch";
import { applyToList, type BoardList } from "@/offline/changes";
import {
  loadBoard,
  loadBoards,
  loadList,
  loadMe,
  loadPending,
  type SavedBoard,
  saveList,
  saveMe,
} from "@/offline/copy";
import { activeUser, setActiveUser } from "@/offline/session";
import { finishSignOut } from "@/offline/signOut";
import { isOffline, startSync, sync } from "@/offline/sync";

export type Favorite = Pick<Board, "id" | "name" | "owner">;

export type Me = {
  id: string;
  email: string;
  name: string;
};

export type Member = Me;

// Thrown when offline and the Offline copy doesn't have what was asked
// for: nobody has signed in on this device yet, or this board was never
// opened here.
export const NOT_OFFLINE = 503;
export type NotOffline = "signed-out" | "board";

function notAvailableOffline(what: NotOffline): never {
  throw data(what, {
    status: NOT_OFFLINE,
    statusText: "Not available offline",
  });
}

const favoritesOf = ({ boards, teamBoards }: BoardList): Favorite[] =>
  [...boards, ...teamBoards]
    .filter((board) => board.favorite)
    .map(({ id, name, owner }) => ({ id, name, owner }));

// The board list as the server has it, with any Pending changes laid on
// top, so boards made offline and not sent yet still show.
async function withPending(userId: string, list: BoardList, me: string) {
  const pending = await loadPending(userId);
  return pending.reduce(
    (list, { change }) => applyToList(list, change, me),
    list,
  );
}

let signedIn: Promise<Me> | null = null;

// Who is signed in, asked once and shared by every loader: the router runs
// parent and child loaders at the same time, so none can rely on another
// having found out first. Online it asks the server and starts syncing
// that person's Pending changes; offline it trusts the Offline copy.
function getMe(): Promise<Me> {
  signedIn ??= findMe().catch((error) => {
    signedIn = null;
    throw error;
  });
  return signedIn;
}

// Forgets who was signed in, for signing in or out.
export function forgetMe() {
  signedIn = null;
}

async function findMe(): Promise<Me> {
  if (isOffline()) {
    const userId = activeUser();
    const me = userId && (await loadMe(userId));
    if (!me) notAvailableOffline("signed-out");
    await startSync(me);
    return me;
  }

  if (await finishSignOut()) throw redirect("/login");

  const res = await apiFetch("/api/me");
  if (res.status === 401) throw redirect("/login");
  throwIfNotOk(res);

  const me: Me = await res.json();
  setActiveUser(me.id);
  await saveMe(me);
  await startSync(me);
  return me;
}

// Whether boardsLoader last read the Offline copy rather than the server.
let boardsLoadedOffline: boolean | null = null;

// The layout's data (favorites, mostly) differs offline, where only boards
// on this device can be opened, so it loads again on the next navigation
// after the connection comes or goes, not only when React Router would.
export const boardsShouldRevalidate: ShouldRevalidateFunction = ({
  defaultShouldRevalidate,
}) => defaultShouldRevalidate || boardsLoadedOffline !== isOffline();

export async function boardsLoader() {
  const me = await getMe();
  boardsLoadedOffline = isOffline();
  if (boardsLoadedOffline) {
    // Only boards on this device can be opened offline.
    const [list, saved] = await Promise.all([
      loadList(me.id),
      loadBoards(me.id),
    ]);
    const onDevice = new Set(saved.map((board) => board.id));
    const favorites = list ? favoritesOf(list) : [];
    return {
      me,
      favorites: favorites.filter((favorite) => onDevice.has(favorite.id)),
    };
  }

  const res = await apiFetch("/api/boards");
  if (res.status === 401) throw redirect("/login");
  throwIfNotOk(res);

  const list = await withPending(me.id, await res.json(), me.id);
  await saveList(me.id, list);
  return { me, favorites: favoritesOf(list) };
}

export type BoardListData = {
  boards: Board[];
  teamBoards: Board[];
  // Offline only: when each board was last saved on this device, which ones
  // can still be changed (Solo boards), and whether some boards in the list
  // aren't on this device at all.
  offline?: {
    savedAt: Record<string, number>;
    solo: Record<string, boolean>;
    missing: boolean;
  };
};

export const boardListLoader = async (): Promise<BoardListData> => {
  const me = await getMe();
  if (isOffline()) return offlineBoardList(me.id);

  await sync();
  const res = await apiFetch("/api/boards");

  if (!res.ok) {
    throw redirect("/login");
  }
  const list = await withPending(me.id, await res.json(), me.id);
  await saveList(me.id, list);
  const boardIds = list.boards.map((board: Board) => board.id);

  return {
    boards: list.boards,
    teamBoards: list.teamBoards.filter(
      (board: Board) => !boardIds.includes(board.id),
    ),
  };
};

async function offlineBoardList(userId: string): Promise<BoardListData> {
  const [list, saved] = await Promise.all([
    loadList(userId),
    loadBoards(userId),
  ]);
  const byId = new Map(saved.map((board) => [board.id, board]));

  // Only boards opened on this device are here; their task counts come from
  // the saved cards, which may be newer than the list.
  const available = (boards: Board[]) =>
    boards.flatMap((board) => {
      const copy = byId.get(board.id);
      if (!copy) return [];
      const tasks = copy.cards.flatMap((card) => card.tasks);
      return [
        {
          ...board,
          name: copy.name,
          description: copy.description,
          taskCount: tasks.length,
          doneCount: tasks.filter((task) => task.status === "done").length,
        },
      ];
    });

  const boards = available(list?.boards ?? []);
  const teamBoards = available(list?.teamBoards ?? []).filter(
    (board) => !boards.some((own) => own.id === board.id),
  );
  const listed = (list?.boards.length ?? 0) + (list?.teamBoards.length ?? 0);

  return {
    boards,
    teamBoards,
    offline: {
      savedAt: Object.fromEntries(saved.map((b) => [b.id, b.savedAt])),
      solo: Object.fromEntries(saved.map((b) => [b.id, b.members.length <= 1])),
      missing: listed > boards.length + teamBoards.length,
    },
  };
}

export type BoardViewData = {
  cardsWithTasks: CardType[];
  name: string;
  description: string;
  owner: string;
  members: Member[];
  // When this was saved on the device, if it comes from the Offline copy.
  savedAt?: number;
};

export async function boardViewLoader({
  params,
}: LoaderFunctionArgs): Promise<BoardViewData> {
  const boardId = params.boardId!;
  const userId = (await getMe()).id;
  const saved = await loadBoard(userId, boardId);

  // The board this device last saw shows at once, rather than waiting on
  // the server: online, the board loads again once its socket opens
  // (useOpenBoard), as it must anyway to catch what was saved before then.
  if (saved) return fromCopy(saved);
  if (isOffline()) notAvailableOffline("board");

  // Changes made offline go first, so the server has as many of them as it
  // can take; those it can't yet are laid on top of what it answers.
  await sync();
  return fromServer(await fetchBoard(userId, boardId));
}

export function fromServer(board: FetchedBoard): BoardViewData {
  return {
    cardsWithTasks: board.cards,
    name: board.name,
    description: board.description,
    owner: board.owner,
    members: board.members,
  };
}

function fromCopy(saved: SavedBoard): BoardViewData {
  return { ...fromServer(saved), savedAt: saved.savedAt };
}

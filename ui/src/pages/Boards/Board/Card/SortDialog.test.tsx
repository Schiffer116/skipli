// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { BoardChange } from "@/offline/changes";

import { BoardContext } from "../BoardContext";
import SortDialog from "./SortDialog";

const toast = vi.hoisted(() => vi.fn());
vi.mock("sonner", () => ({ toast }));

afterEach(cleanup);

const made: BoardChange[] = [];
const make = vi.fn(async (change: BoardChange) => void made.push(change));
const fetch = vi.fn<typeof globalThis.fetch>();

beforeEach(() => {
  made.length = 0;
  make.mockClear();
  toast.mockClear();
  fetch.mockReset();
  vi.stubGlobal("fetch", fetch);
  localStorage.clear();
});

const tasks = ["a", "b", "c"].map((id) => ({
  id,
  cardId: "c1",
  name: id,
  description: "",
  status: "pending",
}));

function open() {
  const router = createMemoryRouter(
    [
      {
        path: "/boards/:boardId",
        element: (
          <BoardContext.Provider
            value={{
              readOnly: false,
              make,
              showCreateCardForm: false,
              setShowCreateCardForm: () => {},
              createTaskFormId: null,
              setCreateTaskFormId: () => {},
            }}
          >
            <SortDialog id="c1" tasks={tasks} />
          </BoardContext.Provider>
        ),
      },
    ],
    { initialEntries: ["/boards/b1"] },
  );
  render(<RouterProvider router={router} />);
}

async function sortBy(criterion: string) {
  const user = userEvent.setup();
  open();
  await user.click(screen.getByRole("button", { name: "Sort tasks" }));
  await user.type(screen.getByLabelText("Sort by"), criterion);
  await user.click(screen.getByRole("button", { name: "Sort" }));
}

describe("Sort tasks", () => {
  it("says what is sent where", async () => {
    open();
    await userEvent.click(screen.getByRole("button", { name: "Sort tasks" }));
    expect(
      screen.getByText(
        "Task names and descriptions are sent to TypeSafe's Jev to sort.",
      ),
    ).toBeTruthy();
  });

  it("asks the server, makes the moves, and offers to undo them", async () => {
    fetch.mockResolvedValue(
      Response.json({ order: ["c", "a", "b"], model: "jev-1.14.0" }),
    );
    await sortBy("most urgent first");

    const [url, init] = fetch.mock.calls[0];
    expect(url).toBe("/api/boards/b1/cards/c1/sort");
    expect(JSON.parse(init!.body as string)).toEqual({
      criterion: "most urgent first",
    });
    expect(made).toEqual([
      {
        type: "moveTask",
        boardId: "b1",
        cardId: "c1",
        taskId: "c",
        newCardId: "c1",
        beforeId: null,
        afterId: "a",
      },
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();

    const [message, options] = toast.mock.calls[0];
    expect(message).toBe('Sorted by "most urgent first"');
    expect(options.action.label).toBe("Undo");
  });

  it("keeps the dialog open with what went wrong, moving nothing", async () => {
    fetch.mockResolvedValue(
      new Response("Jev is unavailable, try again", { status: 503 }),
    );
    await sortBy("most urgent first");

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Jev isn't answering. Try again.",
    );
    expect(make).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Sort" })).toBeTruthy();
  });

  it("is turned off while offline", () => {
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    open();
    const button = screen.getByRole("button", { name: "Sort tasks" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(button.title).toBe("Sorting needs a connection");
    vi.restoreAllMocks();
  });
});

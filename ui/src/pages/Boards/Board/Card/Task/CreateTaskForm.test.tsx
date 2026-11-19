// @vitest-environment jsdom
import { useState } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BoardContext } from "../../BoardContext";
import CreateTaskForm from "./CreateTaskForm";

afterEach(cleanup);

const make = vi.fn(async () => {});
const setCreateTaskFormId = vi.fn();

function open(element: React.ReactNode) {
  const router = createMemoryRouter(
    [
      {
        path: "/boards/:boardId",
        element: (
          <BoardContext.Provider
            value={{
              make,
              showCreateCardForm: false,
              setShowCreateCardForm: () => {},
              createTaskFormId: "c1",
              setCreateTaskFormId,
            }}
          >
            {element}
          </BoardContext.Provider>
        ),
      },
    ],
    { initialEntries: ["/boards/b1"] },
  );
  render(<RouterProvider router={router} />);
}

// Like the board: each card has its own "Create a task" button, and the
// one whose id matches the open form shows the form instead.
function Cards() {
  const [formCardId, setFormCardId] = useState<string | null>("a");
  return (
    <BoardContext.Provider
      value={{
        make,
        showCreateCardForm: false,
        setShowCreateCardForm: () => {},
        createTaskFormId: formCardId,
        setCreateTaskFormId: setFormCardId,
      }}
    >
      <button>Empty space</button>
      {["a", "b"].map((cardId) =>
        formCardId === cardId ? (
          <CreateTaskForm key={cardId} cardId={cardId} />
        ) : (
          <button key={cardId} onClick={() => setFormCardId(cardId)}>
            Create a task in {cardId}
          </button>
        ),
      )}
    </BoardContext.Provider>
  );
}

describe("Create a task", () => {
  it("closes when the user clicks outside the form", async () => {
    const user = userEvent.setup();
    open(
      <>
        <button>Empty space</button>
        <CreateTaskForm cardId="c1" />
      </>,
    );
    await user.type(screen.getByPlaceholderText("Name"), "Half typed");
    await user.click(screen.getByRole("button", { name: "Empty space" }));

    expect(setCreateTaskFormId).toHaveBeenCalledExactlyOnceWith(null);
    expect(make).not.toHaveBeenCalled();
  });

  it("stays open when the user clicks inside the form", async () => {
    const user = userEvent.setup();
    open(<CreateTaskForm cardId="c1" />);
    await user.click(screen.getByPlaceholderText("Name"));
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(setCreateTaskFormId).not.toHaveBeenCalled();
  });

  it("moves to another card when its Create a task is clicked", async () => {
    const user = userEvent.setup();
    open(<Cards />);
    await user.type(screen.getByPlaceholderText("Name"), "Half typed");
    await user.click(
      screen.getByRole("button", { name: "Create a task in b" }),
    );

    expect(screen.getAllByPlaceholderText("Name")).toHaveLength(1);
    expect(screen.getByPlaceholderText("Name")).toHaveProperty("value", "");
    expect(
      screen.getByRole("button", { name: "Create a task in a" }),
    ).toBeTruthy();
  });
});

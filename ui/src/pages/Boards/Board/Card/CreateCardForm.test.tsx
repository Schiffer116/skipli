// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BoardContext } from "../BoardContext";
import CreateCardForm from "./CreateCardForm";

afterEach(cleanup);

const make = vi.fn(async () => {});
const setShowCreateCardForm = vi.fn();

function open() {
  const router = createMemoryRouter(
    [
      {
        path: "/boards/:boardId",
        element: (
          <BoardContext.Provider
            value={{
              make,
              showCreateCardForm: true,
              setShowCreateCardForm,
              createTaskFormId: null,
              setCreateTaskFormId: () => {},
            }}
          >
            <button>Empty space</button>
            <CreateCardForm />
          </BoardContext.Provider>
        ),
      },
    ],
    { initialEntries: ["/boards/b1"] },
  );
  render(<RouterProvider router={router} />);
}

describe("Create a card", () => {
  it("closes when the user clicks outside the form", async () => {
    const user = userEvent.setup();
    open();
    await user.type(screen.getByPlaceholderText("Name"), "Half typed");
    await user.click(screen.getByRole("button", { name: "Empty space" }));

    expect(setShowCreateCardForm).toHaveBeenCalledExactlyOnceWith(false);
    expect(make).not.toHaveBeenCalled();
  });

  it("stays open when the user clicks inside the form", async () => {
    const user = userEvent.setup();
    open();
    await user.click(screen.getByPlaceholderText("Name"));
    await user.click(screen.getByText("Create a card"));

    expect(setShowCreateCardForm).not.toHaveBeenCalled();
  });
});

// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BoardContext } from "../BoardContext";
import EditCardDialog from "./EditCardDialog";
import EditTaskDialog from "./Task/EditTaskDialog";

afterEach(cleanup);

const make = vi.fn(async () => {});

function open(dialog: React.ReactNode) {
  make.mockClear();
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
            {dialog}
          </BoardContext.Provider>
        ),
      },
    ],
    { initialEntries: ["/boards/b1"] },
  );
  render(<RouterProvider router={router} />);
}

// Stops a submit before React sees it.
const holdBack = (e: Event) => {
  e.preventDefault();
  e.stopPropagation();
};

function footerButtons() {
  const footer = screen
    .getByRole("dialog")
    .querySelector("[data-slot=dialog-footer]") as HTMLElement;
  return within(footer)
    .getAllByRole("button")
    .map((b) => b.getAttribute("aria-label") ?? b.textContent);
}

const card = <EditCardDialog id="c1" name="Todo" description="" tasks={[]} />;

describe("Edit card", () => {
  it("saves the new name and description, then closes", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));
    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "Done");
    await user.type(screen.getByLabelText("Description"), "Shipped");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(make).toHaveBeenCalledExactlyOnceWith({
      type: "updateCard",
      boardId: "b1",
      cardId: "c1",
      name: "Done",
      description: "Shipped",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // In a browser the click's own handlers, then React's re-render, run
  // before the form is submitted: a Save that closes the dialog removes the
  // form first, and the browser drops the submit ("form is not connected").
  // jsdom submits before re-rendering, so hold back the submit and check
  // the click alone leaves the form in place.
  it("keeps the form open until it's submitted", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));

    document.addEventListener("submit", holdBack, true);
    await user.click(screen.getByRole("button", { name: "Save" }));
    document.removeEventListener("submit", holdBack, true);

    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(make).not.toHaveBeenCalled();
  });

  it("saves on Enter", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));
    await user.type(screen.getByLabelText("Name"), "!{Enter}");

    expect(make).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps Enter as a line break and saves the lines on Ctrl+Enter", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));
    const description = screen.getByLabelText("Description");
    expect(description.tagName).toBe("TEXTAREA");
    await user.type(description, "Shipped{Enter}Friday");
    expect(make).not.toHaveBeenCalled();
    await user.keyboard("{Control>}{Enter}{/Control}");

    expect(make).toHaveBeenCalledExactlyOnceWith({
      type: "updateCard",
      boardId: "b1",
      cardId: "c1",
      name: "Todo",
      description: "Shipped\nFriday",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("changes nothing on Cancel or Escape", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));
    await user.type(screen.getByLabelText("Name"), "!");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Edit card" }));
    await user.type(screen.getByLabelText("Name"), "!");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();

    expect(make).not.toHaveBeenCalled();
  });

  it("stays open without saving an empty name", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));
    await user.clear(screen.getByLabelText("Name"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(make).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});

describe("Edit card and Edit task", () => {
  it("put Delete first and Save last", async () => {
    const user = userEvent.setup();
    open(card);
    await user.click(screen.getByRole("button", { name: "Edit card" }));
    expect(footerButtons()).toEqual(["Delete card", "Cancel", "Save"]);
    cleanup();

    open(
      <EditTaskDialog
        id="t1"
        cardId="c1"
        name="Milk"
        description=""
        status="pending"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Edit task" }));
    expect(footerButtons()).toEqual(["Delete task", "Cancel", "Save"]);
  });
});

describe("Edit task", () => {
  it("keeps Enter as a line break and saves the lines on Ctrl+Enter", async () => {
    const user = userEvent.setup();
    open(
      <EditTaskDialog
        id="t1"
        cardId="c1"
        name="Milk"
        description=""
        status="pending"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Edit task" }));
    const description = screen.getByLabelText("Description");
    expect(description.tagName).toBe("TEXTAREA");
    await user.type(description, "2%{Enter}Organic");
    await user.keyboard("{Control>}{Enter}{/Control}");

    expect(make).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        type: "updateTask",
        description: "2%\nOrganic",
      }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("only offers Pending or Done as the status", async () => {
    const user = userEvent.setup();
    open(
      <EditTaskDialog
        id="t1"
        cardId="c1"
        name="Milk"
        description=""
        status="pending"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Edit task" }));
    const status = screen.getByLabelText("Status");
    expect(
      within(status)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Pending", "Done"]);
    await user.selectOptions(status, "done");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(make).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ type: "updateTask", status: "done" }),
    );
  });

  it("saves the new name, then closes", async () => {
    const user = userEvent.setup();
    open(
      <EditTaskDialog
        id="t1"
        cardId="c1"
        name="Milk"
        description=""
        status="pending"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Edit task" }));
    document.addEventListener("submit", holdBack, true);
    await user.click(screen.getByRole("button", { name: "Save" }));
    document.removeEventListener("submit", holdBack, true);
    expect(screen.getByRole("dialog")).toBeTruthy();

    await user.type(screen.getByLabelText("Name"), " and eggs");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(make).toHaveBeenCalledExactlyOnceWith({
      type: "updateTask",
      boardId: "b1",
      cardId: "c1",
      taskId: "t1",
      name: "Milk and eggs",
      description: "",
      status: "pending",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

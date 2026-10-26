// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import EditBoardDialog from "./EditBoardDialog";

const make = vi.hoisted(() => vi.fn(async () => "accepted"));
vi.mock("@/offline/pending", () => ({ make }));

afterEach(cleanup);

const onSaved = vi.fn();
const onDeleted = vi.fn();

async function open() {
  make.mockClear();
  onSaved.mockClear();
  onDeleted.mockClear();
  const user = userEvent.setup();
  render(
    <EditBoardDialog
      id="b1"
      name="Home"
      description=""
      onSaved={onSaved}
      onDeleted={onDeleted}
    />,
  );
  await user.click(screen.getByRole("button", { name: "Edit board" }));
  return user;
}

describe("Edit board", () => {
  it("saves the new name and description, then closes", async () => {
    const user = await open();
    await user.type(screen.getByLabelText("Name"), " office");
    await user.type(screen.getByLabelText("Description"), "Chores");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(make).toHaveBeenCalledExactlyOnceWith({
      type: "updateBoard",
      boardId: "b1",
      name: "Home office",
      description: "Chores",
    });
    expect(onSaved).toHaveBeenCalledExactlyOnceWith("Home office", "Chores");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps Enter as a line break and saves the lines on Ctrl+Enter", async () => {
    const user = await open();
    const description = screen.getByLabelText("Description");
    expect(description.tagName).toBe("TEXTAREA");
    await user.type(description, "Chores{Enter}Weekly");
    expect(make).not.toHaveBeenCalled();
    await user.keyboard("{Control>}{Enter}{/Control}");

    expect(make).toHaveBeenCalledExactlyOnceWith({
      type: "updateBoard",
      boardId: "b1",
      description: "Chores\nWeekly",
    });
    expect(onSaved).toHaveBeenCalledExactlyOnceWith("Home", "Chores\nWeekly");
  });

  it("changes nothing on Cancel", async () => {
    const user = await open();
    await user.type(screen.getByLabelText("Name"), "!");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(make).not.toHaveBeenCalled();
  });

  it("deletes once confirmed", async () => {
    const user = await open();
    await user.click(screen.getByRole("button", { name: "Delete board" }));
    await user.click(screen.getByRole("button", { name: "Delete" }));

    expect(make).toHaveBeenCalledExactlyOnceWith({
      type: "deleteBoard",
      boardId: "b1",
    });
    expect(onDeleted).toHaveBeenCalledOnce();
  });

  it("puts Delete first and Save last", async () => {
    await open();
    const footer = screen
      .getByRole("dialog")
      .querySelector("[data-slot=dialog-footer]") as HTMLElement;
    const names = within(footer)
      .getAllByRole("button")
      .map((b) => b.getAttribute("aria-label") ?? b.textContent);
    expect(names).toEqual(["Delete board", "Cancel", "Save"]);
  });
});

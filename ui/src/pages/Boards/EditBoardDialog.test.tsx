// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import EditBoardDialog from "./EditBoardDialog";

const submit = vi.hoisted(() => vi.fn(async () => true));
vi.mock("@/offline/sync", () => ({ submit }));

afterEach(cleanup);

const onSaved = vi.fn();
const onDeleted = vi.fn();

async function open() {
  submit.mockClear();
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

    expect(submit).toHaveBeenCalledExactlyOnceWith({
      type: "updateBoard",
      boardId: "b1",
      name: "Home office",
      description: "Chores",
    });
    expect(onSaved).toHaveBeenCalledExactlyOnceWith("Home office", "Chores");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("changes nothing on Cancel", async () => {
    const user = await open();
    await user.type(screen.getByLabelText("Name"), "!");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(submit).not.toHaveBeenCalled();
  });

  it("deletes once confirmed", async () => {
    const user = await open();
    await user.click(screen.getByRole("button", { name: "Delete board" }));
    await user.click(screen.getByRole("button", { name: "Delete" }));

    expect(submit).toHaveBeenCalledExactlyOnceWith({
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

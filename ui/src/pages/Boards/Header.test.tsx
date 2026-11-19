// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import Header from "./Header";
import type { Favorite, Me } from "./loaders";

vi.mock("@/offline/pending", () => ({
  usePending: () => ({ queued: 0 }),
}));
vi.mock("@/offline/network", () => ({ useOffline: () => false }));
vi.mock("@/offline/signOut", () => ({ signOut: vi.fn() }));
vi.mock("@/utils/theme", () => ({
  readTheme: () => "system",
  setTheme: vi.fn(),
}));

afterEach(cleanup);

const me: Me = { id: "u1", email: "me@example.com", name: "Me" };

const favorite = (id: string, name: string) =>
  ({ id, name, owner: me.id }) as Favorite;

const favorites = [
  favorite("a", "Alpha"),
  favorite("b", "Bravo"),
  favorite("c", "Charlie"),
];

function Where() {
  return <p data-testid="where">{useLocation().pathname}</p>;
}

function renderBar(orderFavorites = vi.fn()) {
  render(
    <MemoryRouter initialEntries={["/boards"]}>
      <Header
        me={me}
        setMe={vi.fn()}
        favorites={favorites}
        orderFavorites={orderFavorites}
      />
      <Where />
    </MemoryRouter>,
  );
  const bar = screen.getByRole("navigation", { name: "Favorite boards" });
  // The accessible name also carries the star's label, so match the end.
  const link = (name: string) =>
    within(bar).getByRole("link", { name: new RegExp(`${name}$`) });
  const where = () => screen.getByTestId("where").textContent;
  return { link, where, orderFavorites };
}

// Presses on `from`, moves by dx/dy, and releases on `to`. A real browser then
// clicks the element under the pointer, which after a drag is often the dragged
// link itself. Returns false when that click was cancelled.
async function drag(
  user: ReturnType<typeof userEvent.setup>,
  from: HTMLElement,
  to: HTMLElement,
  dx: number,
  dy: number,
) {
  await user.pointer([
    { keys: "[MouseLeft>]", target: from, coords: { x: 10, y: 10 } },
    { coords: { x: 10 + dx / 2, y: 10 + dy / 2 }, target: from },
    { coords: { x: 10 + dx, y: 10 + dy }, target: to },
  ]);
  // The release and the click it causes come in one go, as in a browser.
  fireEvent.mouseUp(to, { clientX: 10 + dx, clientY: 10 + dy });
  return fireEvent.click(to);
}

describe("favorites bar", () => {
  it("a plain click still opens the board", async () => {
    const user = userEvent.setup();
    const { link, where, orderFavorites } = renderBar();

    await user.click(link("Bravo"));

    expect(where()).toBe("/boards/b");
    expect(orderFavorites).not.toHaveBeenCalled();
  });

  it("dropping a drag does not navigate, and saves the new order", async () => {
    const user = userEvent.setup();
    const { link, where, orderFavorites } = renderBar();

    // Charlie dragged left onto Alpha's spot, released over Charlie itself.
    const clicked = await drag(user, link("Charlie"), link("Charlie"), -220, 0);

    expect(clicked).toBe(false);
    expect(where()).toBe("/boards");
    expect(orderFavorites).toHaveBeenCalledExactlyOnceWith([
      favorites[2],
      favorites[0],
      favorites[1],
    ]);
  });

  it("swallows only the click that follows a drag", async () => {
    const user = userEvent.setup();
    const { link, where } = renderBar();

    expect(await drag(user, link("Charlie"), link("Charlie"), -220, 0)).toBe(
      false,
    );
    expect(where()).toBe("/boards");

    await user.click(link("Bravo"));
    expect(where()).toBe("/boards/b");
  });
});

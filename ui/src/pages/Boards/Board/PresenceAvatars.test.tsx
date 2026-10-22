// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import PresenceAvatars from "./PresenceAvatars";

vi.mock("@/offline/sync", () => ({ useSync: () => ({ offline: true }) }));

afterEach(cleanup);

const people = (count: number) =>
  Array.from({ length: count }, (_, i) => ({
    id: `u${i}`,
    name: `Person ${i}`,
    email: `p${i}@example.com`,
  }));

describe("Presence avatars", () => {
  it("show nothing when nobody else is here", () => {
    render(<PresenceAvatars people={[]} />);
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("name one person", () => {
    render(<PresenceAvatars people={people(1)} />);
    expect(screen.getAllByRole("button").map((b) => b.ariaLabel)).toEqual([
      "Person 0",
    ]);
  });

  it("fold everyone past the fourth into +N", () => {
    render(<PresenceAvatars people={people(7)} />);
    const labels = screen.getAllByRole("button").map((b) => b.ariaLabel);
    expect(labels).toEqual([
      "Person 0",
      "Person 1",
      "Person 2",
      "Person 3",
      "Person 4, Person 5, Person 6",
    ]);
    expect(screen.getByText("+3")).toBeTruthy();
  });

  it("show exactly four without a +N", () => {
    render(<PresenceAvatars people={people(4)} />);
    expect(screen.getAllByRole("button")).toHaveLength(4);
    expect(screen.queryByText(/^\+/)).toBeNull();
  });
});

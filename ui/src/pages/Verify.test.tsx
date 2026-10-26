// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Verify from "./Verify";

vi.mock("@/offline/session", () => ({ setSignOutPending: vi.fn() }));
vi.mock("./Boards/loaders", () => ({ forgetMe: vi.fn() }));

const fetchMock = vi.fn();

beforeEach(() => {
  sessionStorage.setItem("email", "me@example.com");
  sessionStorage.setItem("session", "old");
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  vi.unstubAllGlobals();
});

const open = () =>
  render(
    <MemoryRouter>
      <Verify />
    </MemoryRouter>,
  );

describe("Verify", () => {
  it("keeps only the first 8 digits", async () => {
    open();
    const input = screen.getByLabelText<HTMLInputElement>("Verification code");
    await userEvent.type(input, "12a3-45 678 9");
    expect(input.value).toBe("12345678");
  });

  it("resends the code and stores the new session", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ session: "new" }), { status: 200 }),
    );
    open();
    await userEvent.click(screen.getByRole("button", { name: "Resend code" }));

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/send",
      expect.objectContaining({
        body: JSON.stringify({ email: "me@example.com" }),
      }),
    );
    expect(await screen.findByText(/new code is on its way/)).toBeTruthy();
    expect(sessionStorage.getItem("session")).toBe("new");
  });

  it("shows the cooldown when resending too soon", async () => {
    fetchMock.mockResolvedValue(new Response("{}", { status: 429 }));
    open();
    await userEvent.click(screen.getByRole("button", { name: "Resend code" }));

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Wait a minute before requesting another.",
    );
    expect(sessionStorage.getItem("session")).toBe("old");
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The edit screen is addressed by user id off the admin list: a spinner
// while the list loads, a plain notice for an id that is not in it, and the
// editor - titled by the username - whose save and cancel both go back.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress }: any) => React.createElement("button", { onClick: onPress }, children),
}));
const params = { current: { id: "u2" } };
const back = vi.fn();
vi.mock("expo-router", () => ({
  Stack: { Screen: (p: any) => React.createElement("div", { "data-testid": "screen-title" }, p.options?.title) },
  useLocalSearchParams: () => params.current,
  useRouter: () => ({ back }),
}));
const refetch = vi.fn();
const users = { data: undefined as unknown, isLoading: false, isError: false, refetch };
vi.mock("@evey/api", () => ({ useUserServiceListUsers: () => users }));
vi.mock("../../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00" } }),
}));
vi.mock("../../../../src/components/layout/HeaderBack", () => ({ HeaderBack: () => null }));
vi.mock("../../../../src/features/admin/EditUserScreen", () => ({
  EditUserScreen: ({ user, onSaved, onCancel }: any) =>
    React.createElement("div", { "data-testid": "editor", "data-user": user.id },
      React.createElement("button", { onClick: onSaved }, "save"),
      React.createElement("button", { onClick: onCancel }, "cancel")),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, fallback?: string) => fallback ?? _k }) }));

async function renderScreen() {
  const mod = await import("./[id]");
  render(React.createElement(mod.default));
}

beforeEach(() => {
  back.mockClear();
  users.data = { users: [{ id: "u1", username: "alice" }, { id: "u2", username: "bob" }] };
  users.isLoading = false;
  users.isError = false;
  refetch.mockClear();
});

describe("admin edit user screen (mobile)", () => {
  it("edits the user named by the route, titled by their username, and goes back on save or cancel", async () => {
    await renderScreen();
    expect(screen.getByTestId("screen-title").textContent).toBe("bob");
    expect(screen.getByTestId("editor").getAttribute("data-user")).toBe("u2");
    fireEvent.click(screen.getByText("save"));
    fireEvent.click(screen.getByText("cancel"));
    expect(back).toHaveBeenCalledTimes(2);
  });

  it("spins while the list loads, with the generic title", async () => {
    users.isLoading = true;
    users.data = undefined;
    await renderScreen();
    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.getByTestId("screen-title").textContent).toBe("Edit User");
    expect(screen.queryByTestId("editor")).toBeNull();
  });

  it("says so when the id is not in the list", async () => {
    users.data = { users: [{ id: "u1", username: "alice" }] };
    await renderScreen();
    expect(screen.getByText("User not found")).toBeTruthy();
    expect(screen.queryByTestId("editor")).toBeNull();
  });

  // A list that failed to load is not a missing user: it used to read
  // "User not found" for a network error, with no way to try again.
  it("offers a retry, not User not found, when the list fails to load", async () => {
    users.data = undefined;
    users.isError = true;
    await renderScreen();
    expect(screen.queryByText("User not found")).toBeNull();
    expect(screen.getByText("Failed to load")).toBeTruthy();
    fireEvent.click(screen.getByText("Retry"));
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

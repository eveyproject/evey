import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityState }: any) => React.createElement("button", { onClick: onPress, "aria-selected": String(!!accessibilityState?.selected) }, children),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#000", textMuted: "#888" }, radii: { sm: 4 } }),
}));
const api = vi.hoisted(() => ({ users: [] as unknown[], profiles: [] as unknown[] }));
vi.mock("@evey/api", () => ({
  useUserServiceListUsers: () => ({ data: { users: api.users } }),
  useUserServiceListAllProfiles: () => ({ data: { profiles: api.profiles } }),
}));

import { WatcherChips } from "./WatcherChips";

beforeEach(() => {
  api.users = [{ id: "u1", username: "alice" }, { id: "u2", username: "bob" }];
  api.profiles = [{ id: "p1", name: "Papa", userId: "u1" }, { id: "p2", name: "Papa", userId: "u2" }, { id: "p3", name: "Kid", userId: "ghost" }];
});

// One chip per profile labelled with its owner (two households can both call
// a profile "Papa"), "everyone" first; the active chip mirrors the value;
// with a single profile there is nothing to filter and the row hides.
describe("WatcherChips", () => {
  it("labels profiles by owner, marks the active one, and reports the picked watcher", () => {
    const onChange = vi.fn();
    render(<WatcherChips value={{ profileId: "p2" }} onChange={onChange} />);
    expect(screen.getByText("Everyone").closest("button")!.getAttribute("aria-selected")).toBe("false");
    expect(screen.getByText("Papa · alice")).toBeTruthy();
    expect(screen.getByText("Papa · bob").closest("button")!.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByText("Kid")).toBeTruthy();
    fireEvent.click(screen.getByText("Papa · alice"));
    expect(onChange).toHaveBeenCalledWith({ profileId: "p1" });
    fireEvent.click(screen.getByText("Everyone"));
    expect(onChange).toHaveBeenCalledWith({});
  });

  it("hides with a single profile", () => {
    api.profiles = [{ id: "p1", name: "Solo", userId: "u1" }];
    const { container } = render(<WatcherChips value={{}} onChange={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });
});

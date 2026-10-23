import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText }: any) =>
    React.createElement("input", { value: value ?? "", onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value) }),
  Alert: { alert: (_t: string, _m: string, buttons: any[]) => buttons[1].onPress() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, f?: any) => (typeof f === "string" ? f : (f?.defaultValue ?? _k)) }),
}));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({
  toast,
  useDemoStore: (sel: (s: { demoMode: boolean; demoLocked: string[] }) => unknown) => sel({ demoMode: false, demoLocked: [] }),
  demoLocks: (s: { demoMode: boolean; demoLocked: string[] }, k: string) => s.demoMode || s.demoLocked.includes(k),
}));
const api = vi.hoisted(() => ({ list: [] as unknown[], add: vi.fn(), remove: vi.fn() }));
vi.mock("@evey/api", () => ({
  useAdminPluginServiceListTrustedPublishers: () => ({ data: { publishers: api.list }, isLoading: false }),
  useAdminPluginServiceAddTrustedPublisher: () => ({ mutate: api.add, isPending: false }),
  useAdminPluginServiceRemoveTrustedPublisher: () => ({ mutate: api.remove, isPending: false }),
  getAdminPluginServiceListTrustedPublishersQueryKey: () => ["pubs"],
  getAdminPluginServiceListPluginsQueryKey: () => ["plugins"],
  apiErrorMessage: (e: Error, _t: unknown, f: string) => e?.message || f,
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", bg: "#000", error: "#f00" },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12 },
  }),
}));

import { TrustedPublishersCard } from "./TrustedPublishersCard";

beforeEach(() => {
  vi.clearAllMocks();
  api.list = [
    { fingerprint: "sha256:aaaa1111bbbb2222", name: "Server operator key", seeded: true },
    { fingerprint: "sha256:cccc3333dddd4444", name: "Acme", seeded: false },
  ];
});

describe("TrustedPublishersCard", () => {
  it("badges the seeded key and offers Remove only on the others", () => {
    render(<TrustedPublishersCard />);
    expect(screen.getByText("Server key")).toBeTruthy();
    expect(screen.getAllByText("Remove")).toHaveLength(1);
  });

  it("adds a publisher by name and pasted PEM", () => {
    render(<TrustedPublishersCard />);
    fireEvent.click(screen.getByText("Add a publisher"));
    const inputs = document.querySelectorAll("input");
    fireEvent.change(inputs[0], { target: { value: "New Pub" } });
    fireEvent.change(inputs[1], { target: { value: "KEY" } });
    fireEvent.click(screen.getAllByText("Add a publisher")[0]);
    expect(api.add).toHaveBeenCalledWith({ data: { name: "New Pub", publicKeyPem: "KEY" } });
  });

  it("removes a publisher by fingerprint after the confirmation", () => {
    render(<TrustedPublishersCard />);
    fireEvent.click(screen.getByText("Remove"));
    expect(api.remove).toHaveBeenCalledWith({ fingerprint: "sha256:cccc3333dddd4444" });
  });
});

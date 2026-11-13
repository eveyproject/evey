import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", { role: "dialog" }, children) : null),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText }: any) =>
    React.createElement("input", { value: value ?? "", onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value) }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, f?: any) => (typeof f === "string" ? f : (f?.defaultValue ?? _k)) }),
}));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (s: { demoMode: boolean; demoLocked: string[] }) => unknown) => sel({ demoMode: false, demoLocked: [] }),
  demoLocks: () => false,
}));
const api = vi.hoisted(() => ({ add: vi.fn() }));
vi.mock("@evey/api", () => ({
  useAdminPluginServiceAddTrustedPublisher: (opts: { mutation: { onSuccess: () => void } }) => ({
    mutate: (vars: unknown) => { api.add(vars); opts.mutation.onSuccess(); },
    isPending: false,
  }),
  getAdminPluginServiceListTrustedPublishersQueryKey: () => ["pubs"],
  getAdminPluginServiceListPluginsQueryKey: () => ["plugins"],
  apiErrorMessage: (e: Error, _t: unknown, f: string) => e?.message || f,
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", bg: "#000", error: "#f00", warning: "#ff0", success: "#0f0" },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12 },
  }),
}));

import { PluginSignatureBadge } from "./PluginSignatureBadge";

const PEM = "-----BEGIN PUBLIC KEY-----\nX\n-----END PUBLIC KEY-----\n";

function trust(name: string) {
  fireEvent.click(screen.getByText("Trust this key"));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: name } });
  fireEvent.click(screen.getByText("Trust"));
}

beforeEach(() => vi.clearAllMocks());

describe("PluginSignatureBadge", () => {
  // The install preview: nothing is installed to hold the key, so the PEM it
  // carries is what gets trusted, and the preview hears the name.
  it("trusts the key the install preview carries, by its PEM", () => {
    const onTrusted = vi.fn();
    render(<PluginSignatureBadge signature={{ verified: true, trusted: false, keyFingerprint: "sha256:ab", keyPem: PEM }} canTrust onTrusted={onTrusted} />);
    trust(" Acme ");
    expect(api.add).toHaveBeenCalledWith({ data: { name: "Acme", publicKeyPem: PEM } });
    expect(onTrusted).toHaveBeenCalledWith("Acme");
  });

  // A plugin card: the server already holds the installed plugin's key.
  it("trusts an installed plugin's key by its fingerprint", () => {
    render(<PluginSignatureBadge signature={{ verified: true, trusted: false, keyFingerprint: "sha256:ab" }} />);
    trust("Acme");
    expect(api.add).toHaveBeenCalledWith({ data: { name: "Acme", fingerprint: "sha256:ab" } });
  });
});

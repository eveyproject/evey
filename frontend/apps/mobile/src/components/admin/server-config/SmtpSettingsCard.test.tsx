import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888" },
    radii: { md: 8 },
  }),
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { success: (...a: unknown[]) => toastSuccess(...a), error: (...a: unknown[]) => toastError(...a) },
}));

const fetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => fetchMock(...a) }));

import { SmtpSettingsCard } from "./SmtpSettingsCard";

beforeEach(() => {
  toastSuccess.mockClear();
  toastError.mockClear();
  fetchMock.mockReset();
});

describe("SmtpSettingsCard", () => {
  it("seeds fields from the entries once they arrive", () => {
    render(<SmtpSettingsCard entries={[{ key: "smtp.host", value: "smtp.example.com" }]} saveEntry={vi.fn()} saving={false} />);
    expect(screen.getByDisplayValue("smtp.example.com")).toBeTruthy();
  });

  it("saves all six SMTP keys and toasts success once all resolve", async () => {
    const saveEntry = vi.fn(async (_vars: { key: string; value: string }) => ({}));
    render(<SmtpSettingsCard entries={[]} saveEntry={saveEntry} saving={false} />);

    fireEvent.click(screen.getByText("Save SMTP Settings"));

    await vi.waitFor(() => expect(toastSuccess).toHaveBeenCalledTimes(1));
    expect(saveEntry).toHaveBeenCalledTimes(6);
    expect(saveEntry).toHaveBeenCalledWith({ key: "smtp.host", value: "" });
    expect(toastError).not.toHaveBeenCalled();
  });

  it("toasts one failure naming the key and no success when one key errors", async () => {
    const saveEntry = vi.fn(async (vars: { key: string; value: string }) => {
      if (vars.key === "smtp.password") throw new Error("boom");
      return {};
    });
    render(<SmtpSettingsCard entries={[]} saveEntry={saveEntry} saving={false} />);

    fireEvent.click(screen.getByText("Save SMTP Settings"));

    await vi.waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));
    expect(toastError).toHaveBeenCalledWith("Failed to save {{key}}");
    expect(toastSuccess).not.toHaveBeenCalled();
  });

  it("disables the save button while saving", () => {
    render(<SmtpSettingsCard entries={[]} saveEntry={vi.fn()} saving />);
    expect(screen.getByTestId("loading")).toBeTruthy();
  });
});

describe("SmtpSettingsCard test e-mail", () => {
  it("posts to the test route and reports the recipient", async () => {
    fetchMock.mockResolvedValue({ to: "admin@example.com" });
    render(<SmtpSettingsCard entries={[]} saveEntry={vi.fn()} saving={false} />);
    fireEvent.click(screen.getByText("Send test email"));
    await vi.waitFor(() => expect(toastSuccess).toHaveBeenCalledWith("admin.serverConfig.testEmailSent"));
    expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/smtp/test", method: "POST", data: {} });
  });

  it("names the missing configuration instead of a generic error", async () => {
    fetchMock.mockRejectedValue({ errorCode: "not_configured" });
    render(<SmtpSettingsCard entries={[]} saveEntry={vi.fn()} saving={false} />);
    fireEvent.click(screen.getByText("Send test email"));
    await vi.waitFor(() => expect(toastError).toHaveBeenCalledWith("admin.serverConfig.testEmailNotConfigured"));
  });
});

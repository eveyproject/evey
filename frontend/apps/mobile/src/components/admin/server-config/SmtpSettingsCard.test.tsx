import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888" },
    radii: { sm: 4, md: 8, lg: 12 },
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
    expect(screen.getByTestId("spinner")).toBeTruthy();
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

describe("SmtpSettingsCard rows", () => {
  it("gives each of the six settings exactly one info button", () => {
    render(<SmtpSettingsCard entries={[]} saveEntry={vi.fn()} saving={false} />);
    const rows = ["host", "port", "tls", "username", "password", "from"];
    for (const id of rows) {
      const row = screen.getByTestId(`setting-row-smtp-${id}`);
      expect(within(row).getAllByTestId("setting-info-button")).toHaveLength(1);
    }
    expect(screen.getAllByTestId("setting-info-button")).toHaveLength(rows.length);
  });

  it("reveals the password on demand", () => {
    render(<SmtpSettingsCard entries={[{ key: "smtp.password", value: "s3cret" }]} saveEntry={vi.fn()} saving={false} />);
    expect((screen.getByDisplayValue("s3cret") as HTMLInputElement).type).toBe("password");
    fireEvent.click(screen.getByLabelText("Show"));
    expect((screen.getByDisplayValue("s3cret") as HTMLInputElement).type).toBe("text");
  });

  it("saves the picked TLS mode with the six writes", async () => {
    const saveEntry = vi.fn(async (_vars: { key: string; value: string }) => ({}));
    render(<SmtpSettingsCard entries={[]} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getByText("SSL/TLS"));
    fireEvent.click(screen.getByTestId("smtp-save-btn"));
    await vi.waitFor(() => expect(toastSuccess).toHaveBeenCalledTimes(1));
    expect(saveEntry).toHaveBeenCalledWith({ key: "smtp.tls", value: "ssl" });
  });
});

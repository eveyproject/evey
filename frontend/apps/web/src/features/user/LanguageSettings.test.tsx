import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LanguageSettings } from "./LanguageSettings";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1", username: "test" } }),
  useUserServiceListProfiles: () => ({ data: { profiles: [{ id: "p1", isDefault: true }] } }),
  useUserServiceUpdateProfile: () => ({ mutate: vi.fn() }),
  getUserServiceListProfilesQueryKey: () => ["profiles"],
}));

vi.mock("@evey/store", () => ({
  useLocaleStore: (selector: (s: { locale: string; setLocale: () => void }) => unknown) =>
    selector({ locale: "en", setLocale: vi.fn() }),
  useAuthStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state = { accessToken: "fake" };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ accessToken: "fake" }) },
  ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("LanguageSettings", () => {
  it("renders language heading and description", () => {
    render(<LanguageSettings />, { wrapper: createWrapper() });
    expect(screen.getAllByText("Language").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Choose your preferred language")).toBeInTheDocument();
  });

  it("renders all language options", () => {
    render(<LanguageSettings />, { wrapper: createWrapper() });
    expect(screen.getByText("English")).toBeInTheDocument();
    expect(screen.getByText("Français")).toBeInTheDocument();
    expect(screen.getByText("Español")).toBeInTheDocument();
    expect(screen.getByText("中文")).toBeInTheDocument();
    expect(screen.getByText("हिन्दी")).toBeInTheDocument();
  });

  it("changes language on selection", async () => {
    const user = userEvent.setup();
    render(<LanguageSettings />, { wrapper: createWrapper() });
    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "fr");
    expect(select).toHaveValue("fr");
  });
});

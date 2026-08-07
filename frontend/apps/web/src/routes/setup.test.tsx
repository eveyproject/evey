import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockSetupMutate = vi.fn();
const mockCreateLibraryMutate = vi.fn();
const mockNavigate = vi.fn();
const mockSetTokens = vi.fn();

// Spreading importOriginal is required, not cosmetic: the page also imports
// the shared defaultRegionForLocale from this package, and a mock that lists
// only hooks hands it back undefined.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn().mockResolvedValue({ setupComplete: false }),
  useAdminSystemServiceSetupWizard: vi.fn(() => ({ mutate: mockSetupMutate, isPending: false })),
  useLibraryServiceCreateLibrary: vi.fn(() => ({ mutate: mockCreateLibraryMutate, isPending: false })),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: vi.fn(() => ({ setTokens: mockSetTokens })),
  useLocaleStore: vi.fn((sel?: (s: { setLocale: (v: string) => void }) => unknown) =>
    sel ? sel({ setLocale: vi.fn() }) : { setLocale: vi.fn() }
  ),
}));

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (opts: { component: React.ComponentType }) => opts,
  isRedirect: vi.fn(() => false),
  redirect: vi.fn(),
  useNavigate: () => mockNavigate,
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getSetupPage() {
  const mod = await import("./setup");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

async function advanceToWelcome() {
  // Step 0 is now the language picker. Click Continue to land on welcome.
  fireEvent.click(screen.getByRole("button", { name: /Continue/i }));
  await waitFor(() => expect(screen.getByText("Welcome to Evey")).toBeInTheDocument());
}

async function advanceToAdmin() {
  await advanceToWelcome();
  fireEvent.click(screen.getByRole("button", { name: /Get Started/i }));
  await waitFor(() => expect(screen.getByText("Create admin account")).toBeInTheDocument());
}

describe("SetupPage", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("starts on language step with a Continue button", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Continue/i })).toBeInTheDocument();
  });

  it("shows step indicator - step 1 of 4", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Step 1 of 4/i)).toBeInTheDocument();
  });

  it("Continue advances to welcome step", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToWelcome();
    expect(screen.getByText(/Step 2 of 4/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Get Started/i })).toBeInTheDocument();
  });

  it("Get Started advances to admin account step", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToAdmin();
    expect(screen.getByText(/Step 3 of 4/i)).toBeInTheDocument();
  });

  it("admin form shows validation error for short username", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToAdmin();
    fireEvent.change(screen.getByLabelText(/Username/i), { target: { value: "ab" } });
    fireEvent.change(screen.getByLabelText(/^Password$/i), { target: { value: "password123" } });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: /Continue/i }));
    await waitFor(() => expect(screen.getByText(/At least 3 characters/i)).toBeInTheDocument());
    expect(mockSetupMutate).not.toHaveBeenCalled();
  });

  it("admin form shows validation error for short password", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToAdmin();
    fireEvent.change(screen.getByLabelText(/Username/i), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText(/^Password$/i), { target: { value: "short" } });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: /Continue/i }));
    await waitFor(() => expect(screen.getByText(/At least 8 characters/i)).toBeInTheDocument());
    expect(mockSetupMutate).not.toHaveBeenCalled();
  });

  it("admin form shows error when passwords do not match", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToAdmin();
    fireEvent.change(screen.getByLabelText(/Username/i), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText(/^Password$/i), { target: { value: "password123" } });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), { target: { value: "different123" } });
    fireEvent.click(screen.getByRole("button", { name: /Continue/i }));
    await waitFor(() => expect(screen.getByText(/Passwords do not match/i)).toBeInTheDocument());
    expect(mockSetupMutate).not.toHaveBeenCalled();
  });

  it("valid admin form calls setup mutation", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToAdmin();
    fireEvent.change(screen.getByLabelText(/Username/i), { target: { value: "admin" } });
    fireEvent.change(screen.getByLabelText(/^Password$/i), { target: { value: "password123" } });
    fireEvent.change(screen.getByLabelText(/Confirm password/i), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: /Continue/i }));
    await waitFor(() => expect(mockSetupMutate).toHaveBeenCalledOnce());
    const callArg = mockSetupMutate.mock.calls[0][0];
    expect(callArg.data.adminUsername).toBe("admin");
    expect(callArg.data.adminPassword).toBe("password123");
  });

  it("Back from admin step returns to welcome", async () => {
    const SetupPage = await getSetupPage();
    render(React.createElement(SetupPage), { wrapper: createWrapper() });
    await advanceToAdmin();
    fireEvent.click(screen.getByRole("button", { name: /^Back$/i }));
    await waitFor(() => expect(screen.getByText("Welcome to Evey")).toBeInTheDocument());
  });
});

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SmartCollectionDialog } from "./SmartCollectionDialog";

const mockCreateMutateAsync = vi.fn();
const mockUpdateMutateAsync = vi.fn();

vi.mock("./api", () => ({
  useCreateCollection: () => ({
    mutateAsync: mockCreateMutateAsync,
    isPending: false,
  }),
  useUpdateCollection: () => ({
    mutateAsync: mockUpdateMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: { open: boolean; children: React.ReactNode; title?: string }) =>
    open ? (
      <div role="dialog" aria-label={title}>
        {title && <h2>{title}</h2>}
        {children}
      </div>
    ) : null,
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading, type }: { children: React.ReactNode; onClick?: () => void; loading?: boolean; type?: string }) => (
    <button onClick={onClick} type={type as "button" | "submit" | "reset"} disabled={loading}>
      {children}
    </button>
  ),
}));

vi.mock("@/components/ui/Input", () => ({
  Input: ({ label, value, onChange, placeholder, error, autoFocus }: {
    label?: string;
    value: string;
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
    placeholder?: string;
    error?: string;
    autoFocus?: boolean;
  }) => (
    <div>
      {label && <label htmlFor={`input-${label}`}>{label}</label>}
      <input
        id={`input-${label}`}
        aria-label={label}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        autoFocus={autoFocus}
      />
      {error && <span role="alert">{error}</span>}
    </div>
  ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("SmartCollectionDialog", () => {
  beforeEach(() => {
    mockCreateMutateAsync.mockReset();
    mockUpdateMutateAsync.mockReset();
    mockCreateMutateAsync.mockResolvedValue({ id: "new-id" });
    mockUpdateMutateAsync.mockResolvedValue(undefined);
  });

  it("renders dialog with title and name field when open", () => {
    render(
      <SmartCollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("New smart collection")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /Name/i })).toBeInTheDocument();
  });

  it("does not render when closed", () => {
    render(
      <SmartCollectionDialog open={false} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows name required error when submitting without name", async () => {
    const user = userEvent.setup();
    render(
      <SmartCollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(screen.getByRole("alert")).toHaveTextContent("Name is required");
    expect(mockCreateMutateAsync).not.toHaveBeenCalled();
  });

  it("shows 'No rules yet' when no rules added", () => {
    render(
      <SmartCollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText(/No rules yet/i)).toBeInTheDocument();
  });

  it("add Genre rule and submit creates collection with correct rules", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    const onClose = vi.fn();

    render(
      <SmartCollectionDialog open={true} onClose={onClose} onSaved={onSaved} />,
      { wrapper: createWrapper() }
    );

    await user.type(screen.getByRole("textbox", { name: /Name/i }), "Action Movies");

    fireEvent.click(screen.getByRole("button", { name: /Genre/i }));

    const genreInput = screen.getByPlaceholderText("e.g. Action");
    fireEvent.change(genreInput, { target: { value: "Action" } });

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Action Movies",
          rules: [{ field: "genre", operator: "eq", value: "Action" }],
        })
      );
    });

    expect(onSaved).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("add Year rule shows operator dropdown and value input", () => {
    render(
      <SmartCollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    fireEvent.click(screen.getByRole("button", { name: /Year/i }));

    expect(screen.getByPlaceholderText("e.g. 2020")).toBeInTheDocument();
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("shows edit title when editing prop is provided", () => {
    const editing = {
      id: "e1",
      name: "Old Filter",
      rules: [],
      createdAt: new Date().toISOString(),
    };

    render(
      <SmartCollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} editing={editing} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("Edit smart collection")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
  });

  it("calls onClose when Cancel is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();

    render(
      <SmartCollectionDialog open={true} onClose={onClose} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
  });

  it("calls updateCollection when editing and saving", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    const onClose = vi.fn();
    const editing = {
      id: "e1",
      name: "Old Filter",
      rules: [{ field: "genre" as const, operator: "eq" as const, value: "Comedy" }],
      createdAt: new Date().toISOString(),
    };

    render(
      <SmartCollectionDialog open={true} onClose={onClose} onSaved={onSaved} editing={editing} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(mockUpdateMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "e1",
          name: "Old Filter",
          rules: [{ field: "genre", operator: "eq", value: "Comedy" }],
        })
      );
    });

    expect(onSaved).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("shows rating_gte and library_id rule buttons", () => {
    render(
      <SmartCollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByRole("button", { name: /Rating/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Library/i })).toBeInTheDocument();
  });
});

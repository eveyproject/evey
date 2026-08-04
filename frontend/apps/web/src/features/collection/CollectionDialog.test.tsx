import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CollectionDialog } from "./CollectionDialog";

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

// Some of the i18n keys this dialog reuses (media.collections.newCollection,
// editCollection, common.description...) predate the test suite's static
// translation snapshot, so a raw-key identity translator keeps this test
// independent of that snapshot's freshness.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
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
  Input: ({ label, value, onChange, error, autoFocus }: {
    label?: string;
    value: string;
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
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

describe("CollectionDialog", () => {
  beforeEach(() => {
    mockCreateMutateAsync.mockReset();
    mockUpdateMutateAsync.mockReset();
    mockCreateMutateAsync.mockResolvedValue({ id: "new-id" });
    mockUpdateMutateAsync.mockResolvedValue(undefined);
  });

  it("renders the create title with an empty name field when open", () => {
    render(
      <CollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("media.collections.newCollection")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "common.name" })).toHaveValue("");
  });

  it("shows name required error when submitting without a name", async () => {
    const user = userEvent.setup();
    render(
      <CollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "common.create" }));

    expect(screen.getByRole("alert")).toHaveTextContent("media.collections.nameRequired");
    expect(mockCreateMutateAsync).not.toHaveBeenCalled();
  });

  // A collection created "smart" without any rule would be rejected by the
  // server (POST /collections requires rules when smart:true) - the manual
  // path must never set it.
  it("creates a manual collection with name and overview, never smart", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    const onClose = vi.fn();

    render(
      <CollectionDialog open={true} onClose={onClose} onSaved={onSaved} />,
      { wrapper: createWrapper() }
    );

    await user.type(screen.getByRole("textbox", { name: "common.name" }), "Family Favorites");
    await user.type(screen.getByLabelText("common.description"), "Movies we watch together");
    await user.click(screen.getByRole("button", { name: "common.create" }));

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledWith({
        name: "Family Favorites",
        overview: "Movies we watch together",
      });
    });
    const call = mockCreateMutateAsync.mock.calls[0][0];
    expect(call).not.toHaveProperty("smart");
    expect(call).not.toHaveProperty("rules");

    expect(onSaved).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("does not send overview on create when the description was left empty", async () => {
    const user = userEvent.setup();
    render(
      <CollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    await user.type(screen.getByRole("textbox", { name: "common.name" }), "Weekend Watchlist");
    await user.click(screen.getByRole("button", { name: "common.create" }));

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledWith({
        name: "Weekend Watchlist",
        overview: undefined,
      });
    });
  });

  describe("editing", () => {
    const editing = { id: "c1", name: "Alien - Saga", overview: "A crew of astronauts is hunted by a deadly lifeform." };

    it("shows the edit title with the existing name and overview pre-filled", () => {
      render(
        <CollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} editing={editing} />,
        { wrapper: createWrapper() }
      );

      expect(screen.getByText("media.collections.editCollection")).toBeInTheDocument();
      expect(screen.getByRole("textbox", { name: "common.name" })).toHaveValue("Alien - Saga");
      expect(screen.getByLabelText("common.description")).toHaveValue(editing.overview);
    });

    // This is the guarantee that protects a saga's TMDB synopsis: renaming a
    // collection must not silently wipe its description.
    it("does not send overview when the description was not touched", async () => {
      const user = userEvent.setup();
      render(
        <CollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} editing={editing} />,
        { wrapper: createWrapper() }
      );

      await user.clear(screen.getByRole("textbox", { name: "common.name" }));
      await user.type(screen.getByRole("textbox", { name: "common.name" }), "Alien Saga");
      await user.click(screen.getByRole("button", { name: "common.save" }));

      await waitFor(() => {
        expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
          id: "c1",
          name: "Alien Saga",
          overview: undefined,
        });
      });
    });

    it("sends the new overview when the description was edited", async () => {
      const user = userEvent.setup();
      render(
        <CollectionDialog open={true} onClose={vi.fn()} onSaved={vi.fn()} editing={editing} />,
        { wrapper: createWrapper() }
      );

      await user.clear(screen.getByLabelText("common.description"));
      await user.type(screen.getByLabelText("common.description"), "Updated synopsis");
      await user.click(screen.getByRole("button", { name: "common.save" }));

      await waitFor(() => {
        expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
          id: "c1",
          name: "Alien - Saga",
          overview: "Updated synopsis",
        });
      });
    });
  });

  it("calls onClose when Cancel is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();

    render(
      <CollectionDialog open={true} onClose={onClose} onSaved={vi.fn()} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "common.cancel" }));

    expect(onClose).toHaveBeenCalled();
  });
});

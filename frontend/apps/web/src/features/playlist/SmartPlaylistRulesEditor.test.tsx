import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SmartPlaylistRulesEditor } from "./SmartPlaylistRulesEditor";

const mockMutateAsync = vi.fn();

vi.mock("./api", () => ({
  useUpdatePlaylist: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({
    children,
    onClick,
    loading,
    type,
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    loading?: boolean;
    type?: string;
  }) => (
    <button
      onClick={onClick}
      type={(type as "button" | "submit" | "reset") ?? "button"}
      disabled={loading}
    >
      {children}
    </button>
  ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const PLAYLIST_ID = "pl-123";

describe("SmartPlaylistRulesEditor", () => {
  beforeEach(() => {
    mockMutateAsync.mockReset();
    mockMutateAsync.mockResolvedValue({});
  });

  it("renders rule list in read-only mode", () => {
    const rules = [
      { field: "genre", operator: "eq", value: "Action" },
      { field: "year", operator: "gte", value: "2020" },
    ];
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={rules} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText(/Genre eq Action/)).toBeInTheDocument();
    expect(screen.getByText(/Year gte 2020/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit Rules" })).toBeInTheDocument();
  });

  it("shows no-rules message when initialRules is empty", () => {
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText(/No rules set/)).toBeInTheDocument();
  });

  it("clicking 'Edit Rules' enters editing mode", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));

    expect(screen.getByRole("button", { name: "Save Rules" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    expect(screen.getByText(/No rules yet/)).toBeInTheDocument();
  });

  it("'Add Genre rule' button adds a genre rule row", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Genre rule" }));

    expect(screen.getByPlaceholderText("e.g. Action")).toBeInTheDocument();
  });

  it("'Add Year rule' button adds a year rule row with operator dropdown", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Year rule" }));

    expect(screen.getByPlaceholderText("e.g. 2020")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Operator" })).toBeInTheDocument();
  });

  it("'Add Type rule' button adds a type rule row with media type select", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Type rule" }));

    const valueSelect = screen.getByRole("combobox", { name: "Value" });
    expect(valueSelect).toBeInTheDocument();
    expect(screen.getByText("Movie")).toBeInTheDocument();
  });

  it("value input changes update the rule", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Genre rule" }));

    const input = screen.getByPlaceholderText("e.g. Action");
    fireEvent.change(input, { target: { value: "Horror" } });

    expect((input as HTMLInputElement).value).toBe("Horror");
  });

  it("removing a rule removes the row", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Genre rule" }));

    expect(screen.getByPlaceholderText("e.g. Action")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove rule" }));

    expect(screen.queryByPlaceholderText("e.g. Action")).not.toBeInTheDocument();
  });

  it("Save Rules calls update mutation with valid rules", async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} onSaved={onSaved} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Genre rule" }));
    fireEvent.change(screen.getByPlaceholderText("e.g. Action"), { target: { value: "Action" } });

    await user.click(screen.getByRole("button", { name: "Save Rules" }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          id: PLAYLIST_ID,
          rules: [{ field: "genre", operator: "eq", value: "Action" }],
        })
      );
    });

    expect(onSaved).toHaveBeenCalled();
  });

  it("Save Rules filters out rules with empty value", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    fireEvent.click(screen.getByRole("button", { name: "Add Genre rule" }));
    // leave value empty

    await user.click(screen.getByRole("button", { name: "Save Rules" }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          id: PLAYLIST_ID,
          rules: [],
        })
      );
    });
  });

  it("Cancel exits editing mode without saving", async () => {
    const user = userEvent.setup();
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={[]} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));
    expect(screen.getByRole("button", { name: "Save Rules" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("button", { name: "Save Rules" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit Rules" })).toBeInTheDocument();
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  it("renders existing rules in editing mode when entering edit", async () => {
    const user = userEvent.setup();
    const rules = [{ field: "genre", operator: "eq", value: "Comedy" }];
    render(
      <SmartPlaylistRulesEditor playlistId={PLAYLIST_ID} initialRules={rules} />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole("button", { name: "Edit Rules" }));

    const input = screen.getByDisplayValue("Comedy");
    expect(input).toBeInTheDocument();
  });
});

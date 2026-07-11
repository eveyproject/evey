import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CreatePlaylistModal } from "./CreatePlaylistModal";

const mockMutateAsync = vi.fn();

vi.mock("./api", () => ({
  useCreatePlaylist: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@evey/store", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock("@/components/ui/Select", () => ({
  Select: ({ label, value, onChange, options }: {
    label?: string;
    value: string;
    onChange: (v: string) => void;
    options: { value: string; label: string }[];
  }) => (
    <div>
      {label && <label>{label}</label>}
      <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
    </div>
  ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const defaultProps = {
  open: true,
  onClose: vi.fn(),
};

describe("CreatePlaylistModal", () => {
  beforeEach(() => {
    mockMutateAsync.mockReset();
    mockMutateAsync.mockResolvedValue({ id: "1", name: "Test" });
  });

  it("renders form fields", () => {
    render(<CreatePlaylistModal {...defaultProps} />, { wrapper: createWrapper() });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Create Playlist")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Description")).toBeInTheDocument();
    expect(screen.getByLabelText("Type")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeInTheDocument();
  });

  it("calls create on submit", async () => {
    const user = userEvent.setup();
    render(<CreatePlaylistModal {...defaultProps} />, { wrapper: createWrapper() });

    await user.type(screen.getByLabelText("Name"), "My Playlist");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(mockMutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "My Playlist",
        type: "manual",
      })
    );
  });

  it("does not submit with empty name", async () => {
    const user = userEvent.setup();
    render(<CreatePlaylistModal {...defaultProps} />, { wrapper: createWrapper() });
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  it("calls onClose on Cancel", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CreatePlaylistModal open={true} onClose={onClose} />, { wrapper: createWrapper() });
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("resets form state when cancelled so reopening shows a blank form", async () => {
    const user = userEvent.setup();
    // onClose is a spy - does not actually toggle `open`, so the modal stays
    // mounted. This lets us check that the form fields are cleared after cancel.
    const onClose = vi.fn();
    render(<CreatePlaylistModal open={true} onClose={onClose} />, { wrapper: createWrapper() });
    const nameInput = screen.getByLabelText("Name");
    await user.type(nameInput, "My Draft Playlist");
    expect(nameInput).toHaveValue("My Draft Playlist");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    // Form state must be cleared even though the modal is still mounted.
    expect(nameInput).toHaveValue("");
    expect(onClose).toHaveBeenCalled();
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AddToPlaylistModal } from "./AddToPlaylistModal";

const mockMutateAsync = vi.fn();
let mockPlaylistData: typeof mockPlaylists = [];

const mockPlaylists = [
  { id: "pl-1", name: "Favorites", description: "My favorites", type: "manual" as const, sort_by: "title", sort_order: "asc", created_at: "", updated_at: "" },
  { id: "pl-2", name: "Watch Later", description: "", type: "manual" as const, sort_by: "title", sort_order: "asc", created_at: "", updated_at: "" },
  { id: "pl-3", name: "Auto Playlist", description: "", type: "smart" as const, sort_by: "title", sort_order: "asc", created_at: "", updated_at: "" },
];

vi.mock("./api", () => ({
  usePlaylistList: () => ({ data: mockPlaylistData, isLoading: false }),
  useAddToPlaylist: () => ({ mutateAsync: mockMutateAsync, isPending: false }),
  useCreatePlaylist: () => ({ mutateAsync: vi.fn().mockResolvedValue({}), isPending: false }),
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
  mediaItemId: "media-123",
};

describe("AddToPlaylistModal", () => {
  beforeEach(() => {
    mockMutateAsync.mockReset();
    mockMutateAsync.mockResolvedValue(undefined);
    mockPlaylistData = mockPlaylists;
  });

  it("renders playlist list", () => {
    render(<AddToPlaylistModal {...defaultProps} />, { wrapper: createWrapper() });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Add to Playlist")).toBeInTheDocument();
    expect(screen.getByText("Favorites")).toBeInTheDocument();
    expect(screen.getByText("Watch Later")).toBeInTheDocument();
    // Smart playlists should not appear
    expect(screen.queryByText("Auto Playlist")).not.toBeInTheDocument();
  });

  it("calls add on select", async () => {
    const user = userEvent.setup();
    render(<AddToPlaylistModal {...defaultProps} />, { wrapper: createWrapper() });

    await user.click(screen.getByText("Favorites"));

    expect(mockMutateAsync).toHaveBeenCalledWith({
      playlistId: "pl-1",
      mediaItemId: "media-123",
    });
  });

  it("shows empty state when no manual playlists", () => {
    mockPlaylistData = [mockPlaylists[2]]; // only smart
    render(<AddToPlaylistModal {...defaultProps} />, { wrapper: createWrapper() });
    expect(screen.getByText("No playlists yet.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create Playlist" })).toBeInTheDocument();
  });

  it("shows New Playlist button", () => {
    render(<AddToPlaylistModal {...defaultProps} />, { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: "+ New Playlist" })).toBeInTheDocument();
  });
});

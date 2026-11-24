import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MusicPlaylistModal } from "./MusicPlaylistModal";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  generate: vi.fn(),
  options: { data: undefined as unknown, isLoading: false },
  optionsArgs: vi.fn(),
}));
vi.mock("@/features/playlist/api", () => ({
  useCreatePlaylist: () => ({ mutateAsync: api.create, isPending: false }),
  useGeneratePlaylist: () => ({ mutateAsync: api.generate, isPending: false }),
  useMixOptions: (...a: unknown[]) => {
    api.optionsArgs(...a);
    return api.options;
  },
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { count?: number }) => (o?.count !== undefined ? `${k}:${o.count}` : k) }),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: React.ReactNode }) => (open ? <div>{children}</div> : null),
}));

function mount() {
  const onCreated = vi.fn();
  const onClose = vi.fn();
  render(<MusicPlaylistModal open onClose={onClose} libraryID="lib-1" onCreated={onCreated} />);
  return { onCreated, onClose };
}

beforeEach(() => {
  vi.clearAllMocks();
  api.options = { data: { genres: [{ name: "Rock", count: 12 }, { name: "Blues", count: 3 }, { name: "Electro", count: 1 }], hasHistory: true }, isLoading: false };
  api.create.mockResolvedValue({ id: "new-1" });
  api.generate.mockResolvedValue({ id: "mix-1", name: "Mix Rock" });
});

describe("MusicPlaylistModal", () => {
  it("creates an empty playlist by name and opens it, with no rule to write", async () => {
    const { onCreated } = mount();
    expect(screen.queryByText(/operator|Opérateur/i)).toBeNull();
    fireEvent.change(screen.getByLabelText("media.playlist.name"), { target: { value: "Dimanche" } });
    fireEvent.click(screen.getByRole("button", { name: "media.playlist.createPlaylist" }));
    await waitFor(() => expect(api.create).toHaveBeenCalledWith({ name: "Dimanche", type: "manual" }));
    expect(onCreated).toHaveBeenCalledWith("new-1");
  });

  it("builds a mix from the ticked styles and length, named after them when left blank", async () => {
    const { onCreated } = mount();
    fireEvent.click(screen.getByTestId("music-playlist-mode-mix"));
    expect(api.optionsArgs).toHaveBeenLastCalledWith("lib-1", true);
    fireEvent.click(screen.getByRole("button", { name: /Rock/ }));
    fireEvent.click(screen.getByRole("button", { name: /Blues/ }));
    fireEvent.click(screen.getByRole("radio", { name: "media.music.trackCount:25" }));
    expect(screen.getByText("media.music.mixFromHistory")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "media.music.mixCreate" }));
    await waitFor(() =>
      expect(api.generate).toHaveBeenCalledWith({ libraryId: "lib-1", name: "Mix Rock, Blues", size: 25, genres: ["Rock", "Blues"] }),
    );
    expect(onCreated).toHaveBeenCalledWith("mix-1");
  });

  it("says so when the styles hold no track, and keeps the form open", async () => {
    api.generate.mockRejectedValueOnce(Object.assign(new Error("x"), { httpStatus: 422 }));
    const { onCreated } = mount();
    fireEvent.click(screen.getByTestId("music-playlist-mode-mix"));
    fireEvent.click(screen.getByRole("button", { name: /Electro/ }));
    fireEvent.click(screen.getByRole("button", { name: "media.music.mixCreate" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("media.music.mixNoTracks"));
    expect(onCreated).not.toHaveBeenCalled();
    expect(screen.getByTestId("music-playlist-modal")).toBeInTheDocument();
  });

  it("tells a profile without history that the mix is random for now", () => {
    api.options = { data: { genres: [], hasHistory: false }, isLoading: false };
    mount();
    fireEvent.click(screen.getByTestId("music-playlist-mode-mix"));
    expect(screen.getByText("media.music.mixNoHistory")).toBeInTheDocument();
    expect(screen.getByText("media.music.mixNoGenres")).toBeInTheDocument();
  });

  it("shows the suggested mix name as editable text and keeps the one typed over it", async () => {
    mount();
    fireEvent.click(screen.getByTestId("music-playlist-mode-mix"));
    fireEvent.click(screen.getByRole("button", { name: /Rock/ }));
    const field = screen.getByLabelText("media.playlist.name") as HTMLInputElement;
    expect(field.value).toBe("Mix Rock");
    fireEvent.change(field, { target: { value: "Pour maman" } });
    fireEvent.click(screen.getByRole("button", { name: /Blues/ }));
    expect(field.value).toBe("Pour maman");
    fireEvent.click(screen.getByRole("button", { name: "media.music.mixCreate" }));
    await waitFor(() => expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({ name: "Pour maman", genres: ["Rock", "Blues"] })));
  });
});

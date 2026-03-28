import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaDetailActions } from "./MediaDetailActions";

const mockNavigate = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: { children: React.ReactNode; to: string; params?: Record<string, string>; [k: string]: unknown }) => {
    const href = params ? Object.entries(params).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) : to;
    return <a href={href} {...rest}>{children}</a>;
  },
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  useLibraryServiceListMediaItems: () => ({ data: undefined }),
  useLibraryServiceGetMediaItem: () => ({ data: undefined }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel: (s: { accessToken: string; profileId: string | null; isAdmin: boolean }) => unknown) =>
    sel({ accessToken: "tok", profileId: "profile-1", isAdmin: false }),
}));

vi.mock("@/hooks/useParentalGate", () => ({
  useParentalGate: () => ({ needsPin: () => false }),
  markUnlocked: vi.fn(),
}));

vi.mock("@/components/ParentalPINDialog", () => ({
  ParentalPINDialog: () => null,
}));

vi.mock("../playback/DownloadButton", () => ({
  DownloadButton: () => <button data-testid="download-btn">Download</button>,
}));

vi.mock("./FavoriteButton", () => ({
  FavoriteButton: () => <button data-testid="favorite-btn">Favorite</button>,
}));

vi.mock("./RatingButtons", () => ({
  RatingButtons: () => <div data-testid="rating-buttons" />,
}));

vi.mock("./StarRating", () => ({
  StarRating: () => <div data-testid="star-rating" />,
}));

vi.mock("../playlist/AddToPlaylistModal", () => ({
  AddToPlaylistModal: () => null,
}));

vi.mock("./MediaDetailTrailer", () => ({
  TrailerSection: () => null,
}));

vi.mock("./MediaDetailSeries", () => ({
  PlaySeriesButton: ({ onPlay }: { onPlay: (id: string) => void }) => (
    <button data-testid="play-series-btn" onClick={() => onPlay("ep-1")}>Play Series</button>
  ),
}));

vi.mock("./MediaDetailEdit", () => ({
  MetadataEditPanel: () => <div data-testid="metadata-edit" />,
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, ...props }: { children: React.ReactNode; [k: string]: unknown }) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div data-testid="modal">{children}</div> : null,
}));

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

const baseItem = {
  id: "m1",
  title: "Inception",
  overview: "A thief who steals secrets through dreams.",
  tagline: "Your mind is the scene of the crime.",
  type: "MEDIA_ITEM_TYPE_MOVIE" as const,
  genres: ["Action"],
  studios: ["Warner Bros."],
  filePath: "/movies/inception.mkv",
  people: [],
};

const directors = [{ id: "d1", name: "Christopher Nolan", type: "PERSON_TYPE_DIRECTOR" as const }];
const writers = [{ id: "w1", name: "Jonathan Nolan", type: "PERSON_TYPE_WRITER" as const }];

const defaultProps = {
  item: baseItem,
  isSeries: false,
  isSeason: false,
  directors,
  writers,
  editing: false,
  setEditing: vi.fn(),
  onMetadataUpdated: vi.fn(),
  streamToken: "tok-123",
};

describe("MediaDetailActions", () => {
  it("renders tagline", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByText("Your mind is the scene of the crime.")).toBeInTheDocument();
  });

  it("renders overview", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("media-overview")).toHaveTextContent("A thief who steals secrets through dreams.");
  });

  it("renders directors", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByText("Christopher Nolan")).toBeInTheDocument();
  });

  it("renders studios", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByText(/Warner Bros\./)).toBeInTheDocument();
  });

  it("shows play button for movie", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("play-button")).toBeInTheDocument();
  });

  it("hides play button for series", () => {
    render(<MediaDetailActions {...defaultProps} isSeries />, { wrapper: Wrapper });
    expect(screen.queryByTestId("play-button")).not.toBeInTheDocument();
  });

  it("hides play button for season", () => {
    render(<MediaDetailActions {...defaultProps} isSeason />, { wrapper: Wrapper });
    expect(screen.queryByTestId("play-button")).not.toBeInTheDocument();
  });

  it("shows PlaySeriesButton for series", () => {
    render(<MediaDetailActions {...defaultProps} isSeries />, { wrapper: Wrapper });
    expect(screen.getByTestId("play-series-btn")).toBeInTheDocument();
  });

  it("shows mark-watched button", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("mark-watched-btn")).toBeInTheDocument();
  });

  it("shows download button when filePath present", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("download-btn")).toBeInTheDocument();
  });

  it("hides download button when no filePath", () => {
    const item = { ...baseItem, filePath: undefined };
    render(<MediaDetailActions {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("download-btn")).not.toBeInTheDocument();
  });

  it("shows favorite button", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("favorite-btn")).toBeInTheDocument();
  });

  it("shows rating buttons", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("rating-buttons")).toBeInTheDocument();
  });

  it("shows edit modal when editing=true", () => {
    render(<MediaDetailActions {...defaultProps} editing />, { wrapper: Wrapper });
    expect(screen.getByTestId("modal")).toBeInTheDocument();
    expect(screen.getByTestId("metadata-edit")).toBeInTheDocument();
  });

  it("hides edit modal when editing=false", () => {
    render(<MediaDetailActions {...defaultProps} editing={false} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
  });

  it("hides tagline when missing", () => {
    const item = { ...baseItem, tagline: undefined };
    render(<MediaDetailActions {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(screen.queryByText("Your mind is the scene of the crime.")).not.toBeInTheDocument();
  });

  it("hides directors when empty", () => {
    render(<MediaDetailActions {...defaultProps} directors={[]} />, { wrapper: Wrapper });
    expect(screen.queryByText("Christopher Nolan")).not.toBeInTheDocument();
    expect(screen.getByText("Jonathan Nolan")).toBeInTheDocument();
  });

  it("navigates to player on play click", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByTestId("play-button"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/player/$mediaId", params: { mediaId: "m1" } });
  });
});

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MetadataEditPanel } from "./MediaDetailEdit";
import type { V1MediaItem } from "@evey/api";

const mockSetOverride = { mutateAsync: vi.fn().mockResolvedValue({}), isError: false };
const mockRemoveOverride = { mutateAsync: vi.fn().mockResolvedValue({}) };
const mockRefresh = { mutateAsync: vi.fn().mockResolvedValue({}) };
const mockListOverrides = vi.fn().mockReturnValue({ data: { overrides: [] } });
const mockEveyFetch = vi.fn().mockResolvedValue({ mediaItemId: "m1", matched: true, newTitle: "Some Series" });

// Partial mock: the data hooks are stubbed, but pure helpers keep their real
// implementation - niceNameFromPath feeds a field whose exact text is the point,
// so stubbing it would test the stub.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useMetadataServiceSetMetadataOverride: () => mockSetOverride,
  useMetadataServiceRemoveMetadataOverride: () => mockRemoveOverride,
  useMetadataServiceRefreshMetadata: () => mockRefresh,
  useMetadataServiceGetArtwork: () => ({ data: undefined }),
  useMetadataServiceListMetadataOverrides: (...args: unknown[]) => mockListOverrides(...args),
  getMetadataServiceListMetadataOverridesQueryKey: (id: string) => [`/api/v1/metadata/${id}/overrides`],
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

const baseItem: V1MediaItem = {
  id: "m1",
  title: "Some Movie",
  originalTitle: "Some Movie (Original)",
  year: 2010,
  overview: "Dreams within dreams.",
  genres: ["Action", "Sci-Fi"],
  communityRating: 8.8,
  type: "MEDIA_ITEM_TYPE_MOVIE",
  providerIds: { imdb: "tt0000001", tmdb: "900001" },
};

const defaultProps = {
  item: baseItem,
  onSave: vi.fn(),
  onCancel: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("MetadataEditPanel", () => {
  it("renders a form", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(document.querySelector("form")).toBeTruthy();
  });

  it("renders all FIELD_MAP fields with labels", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const form = document.querySelector("form")!;
    expect(form.querySelectorAll("input, textarea").length).toBeGreaterThanOrEqual(6);
  });

  it("pre-fills title input", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const input = screen.getByDisplayValue("Some Movie");
    expect(input).toBeInTheDocument();
  });

  it("pre-fills year input", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByDisplayValue("2010")).toBeInTheDocument();
  });

  it("pre-fills overview textarea", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByDisplayValue("Dreams within dreams.")).toBeInTheDocument();
  });

  it("pre-fills genres as comma-separated string", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByDisplayValue("Action, Sci-Fi")).toBeInTheDocument();
  });

  it("pre-fills communityRating", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByDisplayValue("8.8")).toBeInTheDocument();
  });

  it("pre-fills IMDB and TMDB ids", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByDisplayValue("tt0000001")).toBeInTheDocument();
    expect(screen.getByDisplayValue("900001")).toBeInTheDocument();
  });

  it("renders IMDB link when imdbId present", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const link = screen.getByRole("link", { name: /imdb/i });
    expect(link).toHaveAttribute("href", "https://www.imdb.com/title/tt0000001/");
  });

  it("renders TMDB link when tmdbId present", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const link = screen.getByRole("link", { name: /tmdb/i });
    expect(link).toHaveAttribute("href", "https://www.themoviedb.org/movie/900001");
  });

  it("renders save, refresh, and cancel buttons", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const buttons = screen.getAllByRole("button");
    expect(buttons.length).toBeGreaterThanOrEqual(3);
  });

  it("calls onCancel when cancel button clicked", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const cancelBtn = screen.getAllByRole("button").find((b) =>
      b.textContent?.toLowerCase().includes("cancel"),
    );
    expect(cancelBtn).toBeTruthy();
    fireEvent.click(cancelBtn!);
    expect(defaultProps.onCancel).toHaveBeenCalled();
  });

  it("shows auto badge when no overrides", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const badges = screen.getAllByText(/auto/i);
    expect(badges.length).toBeGreaterThanOrEqual(1);
  });

  it("handles empty providerIds gracefully", () => {
    const item = { ...baseItem, providerIds: undefined };
    render(<MetadataEditPanel {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(screen.getByDisplayValue("Some Movie")).toBeInTheDocument();
  });

  // ── Filename on disk ────────────────────────────────────────────────────────

  // When a match is wrong, every other field on this panel holds the WRONG
  // film's data. The filename is the only evidence left untouched, so it is
  // shown read-only at the top - and it is what the TMDB identify box should
  // start from, rather than the stored (possibly wrong) title.
  it("shows the cleaned filename, read-only", () => {
    const item = { ...baseItem, filePath: "/media/films/Rocket 2 - 1080P.mkv" };
    render(<MetadataEditPanel {...defaultProps} item={item} />, { wrapper: Wrapper });

    const field = screen.getByTestId("media-edit-filename") as HTMLInputElement;
    expect(field.value).toBe("Rocket 2");
    expect(field).toBeDisabled();
  });

  it("omits the filename field when the path yields nothing usable", () => {
    const item = { ...baseItem, filePath: "" };
    render(<MetadataEditPanel {...defaultProps} item={item} />, { wrapper: Wrapper });

    expect(screen.queryByTestId("media-edit-filename")).not.toBeInTheDocument();
  });

  // ── Custom artwork upload ───────────────────────────────────────────────────

  // The picker used to render nothing when there was fewer than two images to
  // choose from - which is exactly the item an admin needs to upload one for.
  it("offers poster and backdrop upload even with no artwork to choose from", () => {
    render(<MetadataEditPanel {...defaultProps} item={baseItem} />, { wrapper: Wrapper });

    expect(screen.getByTestId("upload-poster")).toBeInTheDocument();
    expect(screen.getByTestId("upload-backdrop")).toBeInTheDocument();
  });

  // Only the three formats the server stores. Notably absent: SVG, which is an
  // image by MIME type and a script container in practice.
  it("restricts the file picker to the formats the server accepts", () => {
    render(<MetadataEditPanel {...defaultProps} item={baseItem} />, { wrapper: Wrapper });

    const input = screen.getByTestId("upload-poster") as HTMLInputElement;
    expect(input.accept).toBe("image/jpeg,image/png,image/webp");
    expect(input.accept).not.toContain("svg");
  });

  it("handles missing optional fields", () => {
    const item: V1MediaItem = {
      id: "m2",
      title: undefined,
      type: "MEDIA_ITEM_TYPE_MOVIE",
    };
    render(<MetadataEditPanel {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(document.querySelector("form")).toBeTruthy();
  });

  it("uses tv mediaType for series", () => {
    const item = { ...baseItem, type: "MEDIA_ITEM_TYPE_SERIES" as const };
    render(<MetadataEditPanel {...defaultProps} item={item} />, { wrapper: Wrapper });
    const tmdbLink = screen.getByRole("link", { name: /tmdb/i });
    expect(tmdbLink).toHaveAttribute("href", "https://www.themoviedb.org/tv/900001");
  });

  it("shows id-changed warning when tmdbId modified", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    const tmdbInput = screen.getByDisplayValue("900001");
    fireEvent.change(tmdbInput, { target: { value: "99999" } });
    const tmdbLink = screen.getByRole("link", { name: /tmdb/i });
    expect(tmdbLink).toHaveAttribute("href", "https://www.themoviedb.org/movie/99999");
  });
});

describe("FieldBadge", () => {
  it("shows overridden badge when override exists", () => {
    mockListOverrides.mockReturnValueOnce({
      data: { overrides: [{ id: "ov1", fieldName: "title", customValue: "Custom" }] },
    });
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getAllByText(/overrid/i).length).toBeGreaterThanOrEqual(1);
  });
});

describe("Re-match metadata", () => {
  it("renders the re-match button", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("media-edit-rematch-btn")).toBeInTheDocument();
  });

  it("opens a confirmation modal when clicked", () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByTestId("media-edit-rematch-btn"));
    expect(screen.getByTestId("media-edit-rematch-confirm-btn")).toBeInTheDocument();
  });

  it("calls eveyFetch with the research endpoint on confirm", async () => {
    render(<MetadataEditPanel {...defaultProps} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByTestId("media-edit-rematch-btn"));
    fireEvent.click(screen.getByTestId("media-edit-rematch-confirm-btn"));
    await waitFor(() => {
      expect(mockEveyFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/metadata/m1/research",
          method: "POST",
        }),
      );
    });
  });

  it("invokes onSave after a successful re-match", async () => {
    const onSave = vi.fn();
    render(<MetadataEditPanel {...defaultProps} onSave={onSave} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByTestId("media-edit-rematch-btn"));
    fireEvent.click(screen.getByTestId("media-edit-rematch-confirm-btn"));
    await waitFor(() => expect(onSave).toHaveBeenCalled());
  });
});

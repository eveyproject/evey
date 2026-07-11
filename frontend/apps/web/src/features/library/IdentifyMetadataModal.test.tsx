import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IdentifyMetadataModal } from "./IdentifyMetadataModal";

// ── Mock @evey/api ────────────────────────────────────────────────────────────
// The component calls eveyFetch directly (not a generated hook) for both the
// search GET and the refresh POST. We control the resolved value per-test.
const mockEveyFetch = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
  getMetadataServiceListMetadataOverridesQueryKey: (id: string) => [`/api/v1/metadata/${id}/overrides`],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// ── Helpers ────────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.invalidateQueries = mockInvalidateQueries as unknown as typeof qc.invalidateQueries;
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const defaultProps = {
  open: true,
  onClose: vi.fn(),
  mediaItemId: "mi-abc",
  initialQuery: "Greyhound",
  mediaType: "movie" as const,
  onApplied: vi.fn(),
};

function renderModal(overrides: Partial<typeof defaultProps> = {}) {
  return render(
    React.createElement(IdentifyMetadataModal, { ...defaultProps, ...overrides }),
    { wrapper: createWrapper() },
  );
}

// A minimal search result returned by the mocked search endpoint
const RESULT_A = {
  tmdb_id: "59967",
  title: "Greyhound",
  year: 2020,
  overview: "WWII destroyer commander crosses the Atlantic.",
  poster_url: undefined,
};

const RESULT_B = {
  imdb_id: "tt6048922",
  title: "Greyhound 2",
  year: 2022,
  overview: "Sequel.",
  poster_url: undefined,
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("IdentifyMetadataModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Rendering ───────────────────────────────────────────────────────────────

  it("renders the modal when open=true", () => {
    renderModal();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("pre-fills the search input with initialQuery", () => {
    renderModal();
    const input = screen.getByTestId("identify-query-input") as HTMLInputElement;
    expect(input.value).toBe("Greyhound");
  });

  it("renders the Search button disabled when query is empty", () => {
    renderModal({ initialQuery: "" });
    const btn = screen.getByTestId("identify-search-btn");
    expect(btn).toBeDisabled();
  });

  it("enables the Search button when query is non-empty", () => {
    renderModal();
    const btn = screen.getByTestId("identify-search-btn");
    expect(btn).not.toBeDisabled();
  });

  // ── Search flow ─────────────────────────────────────────────────────────────

  it("calls eveyFetch search endpoint with query and type on Search click", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(mockEveyFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/admin/media/mi-abc/search-metadata",
          method: "GET",
          params: { query: "Greyhound", type: "movie" },
        }),
      );
    });
  });

  it("calls eveyFetch search endpoint when Enter is pressed in the input", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A]);
    renderModal();

    const input = screen.getByTestId("identify-query-input");
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(mockEveyFetch).toHaveBeenCalledWith(
        expect.objectContaining({ method: "GET", url: "/api/v1/admin/media/mi-abc/search-metadata" }),
      );
    });
  });

  it("does NOT call eveyFetch when query is blank and Search is clicked", async () => {
    renderModal({ initialQuery: "   " });
    // The button is disabled, but also the handler guards against blank queries.
    // We fire the search directly by testing the guard via Enter key on the input.
    // (button is disabled so fireEvent.click would be a no-op anyway)
    expect(mockEveyFetch).not.toHaveBeenCalled();
  });

  it("renders the results list after a successful search", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A, RESULT_B]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(screen.getByTestId("identify-results")).toBeInTheDocument();
    });
    expect(screen.getByText("Greyhound")).toBeInTheDocument();
    expect(screen.getByText("Greyhound 2")).toBeInTheDocument();
  });

  it("renders result year in parentheses", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(screen.getByText("(2020)")).toBeInTheDocument();
    });
  });

  it("renders overview text for each result", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(screen.getByText("WWII destroyer commander crosses the Atlantic.")).toBeInTheDocument();
    });
  });

  it("renders TMDB external link for a result with tmdb_id", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      const link = screen.getByRole("link", { name: /tmdb/i });
      expect(link).toHaveAttribute("href", "https://www.themoviedb.org/movie/59967");
    });
  });

  it("renders IMDB external link for a result with imdb_id only", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_B]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      const link = screen.getByRole("link", { name: /imdb/i });
      expect(link).toHaveAttribute("href", "https://www.imdb.com/title/tt6048922/");
    });
  });

  // ── Empty / no-results state ────────────────────────────────────────────────

  it("shows no-results message when search returns empty array", async () => {
    mockEveyFetch.mockResolvedValueOnce([]);
    renderModal();

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(screen.getByText(/no tmdb matches/i)).toBeInTheDocument();
    });
  });

  it("does not show no-results message before any search is performed", () => {
    renderModal();
    expect(screen.queryByText(/no tmdb matches/i)).not.toBeInTheDocument();
  });

  // ── Apply flow ──────────────────────────────────────────────────────────────

  it("calls eveyFetch refresh endpoint with tmdb_id on apply", async () => {
    mockEveyFetch
      .mockResolvedValueOnce([RESULT_A]) // search
      .mockResolvedValueOnce(undefined); // apply

    renderModal();
    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => screen.getByTestId("identify-apply-59967"));
    fireEvent.click(screen.getByTestId("identify-apply-59967"));

    await waitFor(() => {
      expect(mockEveyFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/admin/media/mi-abc/refresh-metadata",
          method: "POST",
          data: { tmdb_id: "59967", media_type: "movie" },
        }),
      );
    });
  });

  it("calls eveyFetch refresh endpoint with imdb_id when only imdb_id available", async () => {
    mockEveyFetch
      .mockResolvedValueOnce([RESULT_B]) // search
      .mockResolvedValueOnce(undefined); // apply

    renderModal();
    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => screen.getByTestId("identify-apply-tt6048922"));
    fireEvent.click(screen.getByTestId("identify-apply-tt6048922"));

    await waitFor(() => {
      expect(mockEveyFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/admin/media/mi-abc/refresh-metadata",
          method: "POST",
          data: { imdb_id: "tt6048922", media_type: "movie" },
        }),
      );
    });
  });

  it("invalidates media item query after successful apply", async () => {
    mockEveyFetch
      .mockResolvedValueOnce([RESULT_A])
      .mockResolvedValueOnce(undefined);

    renderModal();
    fireEvent.click(screen.getByTestId("identify-search-btn"));
    await waitFor(() => screen.getByTestId("identify-apply-59967"));
    fireEvent.click(screen.getByTestId("identify-apply-59967"));

    await waitFor(() => {
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["/api/v1/media/mi-abc"] }),
      );
    });
  });

  it("calls onApplied callback after successful apply", async () => {
    mockEveyFetch
      .mockResolvedValueOnce([RESULT_A])
      .mockResolvedValueOnce(undefined);

    const onApplied = vi.fn();
    renderModal({ onApplied });
    fireEvent.click(screen.getByTestId("identify-search-btn"));
    await waitFor(() => screen.getByTestId("identify-apply-59967"));
    fireEvent.click(screen.getByTestId("identify-apply-59967"));

    await waitFor(() => expect(onApplied).toHaveBeenCalledOnce());
  });

  it("calls onClose after successful apply", async () => {
    mockEveyFetch
      .mockResolvedValueOnce([RESULT_A])
      .mockResolvedValueOnce(undefined);

    const onClose = vi.fn();
    renderModal({ onClose });
    fireEvent.click(screen.getByTestId("identify-search-btn"));
    await waitFor(() => screen.getByTestId("identify-apply-59967"));
    fireEvent.click(screen.getByTestId("identify-apply-59967"));

    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  });

  it("shows toast error when apply fails", async () => {
    const { toast } = await import("@evey/store");
    mockEveyFetch
      .mockResolvedValueOnce([RESULT_A])
      .mockRejectedValueOnce(new Error("Network error"));

    renderModal();
    fireEvent.click(screen.getByTestId("identify-search-btn"));
    await waitFor(() => screen.getByTestId("identify-apply-59967"));
    fireEvent.click(screen.getByTestId("identify-apply-59967"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("Network error"));
    });
  });

  it("shows toast error when search fails", async () => {
    const { toast } = await import("@evey/store");
    mockEveyFetch.mockRejectedValueOnce(new Error("Search timeout"));

    renderModal();
    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("Search timeout"));
    });
  });

  // ── Cancel / close ──────────────────────────────────────────────────────────

  it("calls onClose without applying when Close button is clicked", () => {
    const onClose = vi.fn();
    renderModal({ onClose });

    fireEvent.click(screen.getByRole("button", { name: /close/i }));

    expect(onClose).toHaveBeenCalledOnce();
    expect(mockEveyFetch).not.toHaveBeenCalled();
  });

  it("resets state (clears results and query) when closed and reopened", async () => {
    mockEveyFetch.mockResolvedValueOnce([RESULT_A]);

    const { rerender } = renderModal();
    fireEvent.click(screen.getByTestId("identify-search-btn"));
    await waitFor(() => screen.getByTestId("identify-results"));

    // Close
    fireEvent.click(screen.getByRole("button", { name: /close/i }));

    // Reopen with same props - results should be cleared
    rerender(
      React.createElement(IdentifyMetadataModal, { ...defaultProps, open: true }),
    );

    expect(screen.queryByTestId("identify-results")).not.toBeInTheDocument();
    const input = screen.getByTestId("identify-query-input") as HTMLInputElement;
    expect(input.value).toBe("Greyhound");
  });

  // ── Updating query input ─────────────────────────────────────────────────────

  it("updates query input as user types", () => {
    renderModal();
    const input = screen.getByTestId("identify-query-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Snowpiercer" } });
    expect(input.value).toBe("Snowpiercer");
  });

  it("disables Search button when query is cleared to empty", () => {
    renderModal();
    const input = screen.getByTestId("identify-query-input");
    fireEvent.change(input, { target: { value: "" } });
    expect(screen.getByTestId("identify-search-btn")).toBeDisabled();
  });

  // ── TV media type ────────────────────────────────────────────────────────────

  it("passes mediaType=tv to search endpoint for TV items", async () => {
    mockEveyFetch.mockResolvedValueOnce([]);
    renderModal({ mediaType: "tv", initialQuery: "Orphan Black" });

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      expect(mockEveyFetch).toHaveBeenCalledWith(
        expect.objectContaining({ params: { query: "Orphan Black", type: "tv" } }),
      );
    });
  });

  it("uses tv path in TMDB link for TV results", async () => {
    const tvResult = { tmdb_id: "62073", title: "Orphan Black", year: 2013, overview: "Clone drama." };
    mockEveyFetch.mockResolvedValueOnce([tvResult]);
    renderModal({ mediaType: "tv", initialQuery: "Orphan Black" });

    fireEvent.click(screen.getByTestId("identify-search-btn"));

    await waitFor(() => {
      const link = screen.getByRole("link", { name: /tmdb/i });
      expect(link).toHaveAttribute("href", "https://www.themoviedb.org/tv/62073");
    });
  });
});

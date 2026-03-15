import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockListProfiles = vi.fn();
const mockEveyFetch = vi.fn();
const mockInvalidateQueries = vi.fn();

// Both @evey/api and @evey/api/src/fetcher alias to the same file in vitest config,
// so we mock under @evey/api/src/fetcher and include everything the component needs.
vi.mock("@evey/api/src/fetcher", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
  useTranscodingServiceListTranscodeProfiles: (...args: unknown[]) => mockListProfiles(...args),
  getTranscodingServiceListTranscodeProfilesQueryKey: () => ["/api/v1/admin/transcoding/profiles"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});


function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getPage() {
  const mod = await import("./transcoding-profiles.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockProfile = {
  id: "prof-1",
  name: "HD 1080p",
  maxWidth: 1920,
  maxHeight: 1080,
  videoCodec: "H.264",
  audioCodec: "AAC",
  maxBitrate: "8000",
  hwAccel: "HW_ACCEL_TYPE_NONE",
  isDefault: true,
};

describe("TranscodingProfilesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockEveyFetch.mockResolvedValue({});
  });

  it("renders page heading", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Transcoding Profiles")).toBeInTheDocument();
  });

  it("shows empty message when no profiles", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText(/No transcoding profiles found/i)).toBeInTheDocument();
  });

  it("renders profile row with name and default badge", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("HD 1080p")).toBeInTheDocument();
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByText("1080p")).toBeInTheDocument();
  });

  it("Create Profile button opens modal", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    // Click the header Create Profile button (not table row button)
    const createBtns = screen.getAllByRole("button", { name: /Create Profile/i });
    fireEvent.click(createBtns[0]);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /^Create$/i })).toBeInTheDocument();
  });

  it("Edit button opens modal with profile data", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Edit$/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText("Edit Profile")).toBeInTheDocument();
  });

  it("Delete button opens confirmation dialog", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    fireEvent.click(screen.getAllByRole("button", { name: /^Delete$/i })[0]);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText("Delete Profile")).toBeInTheDocument();
  });

  it("Cancel in delete dialog does not call API", async () => {
    mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    fireEvent.click(screen.getAllByRole("button", { name: /^Delete$/i })[0]);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockEveyFetch).not.toHaveBeenCalled();
  });
});

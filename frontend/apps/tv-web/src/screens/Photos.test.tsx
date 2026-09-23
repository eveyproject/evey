import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";

const { libraries, albums, photos, navigateMock, goBackMock } = vi.hoisted(() => ({
  libraries: vi.fn(), albums: vi.fn(), photos: vi.fn(), navigateMock: vi.fn(), goBackMock: vi.fn(),
}));

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
  useTvKeys: () => {},
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a), goBack: goBackMock }));
const authState = { accessToken: "tok" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  getBaseURL: () => "http://server",
  useLibraryServiceListLibraries: () => libraries(),
  usePhotoServiceListPhotoAlbums: () => albums(),
  usePhotoServiceListPhotos: (...a: unknown[]) => photos(...a),
}));

import { PhotosScreen, SlideshowScreen } from "./Photos";

const PHOTOS = [
  { id: "p-1", title: "One" },
  { id: "p-2", title: "Two" },
  { id: "p-3", title: "Three" },
];

beforeEach(() => {
  navigateMock.mockReset();
  libraries.mockReturnValue({ data: { libraries: [{ id: "lib-photos", name: "Photos", type: "LIBRARY_TYPE_PHOTOS" }] } });
  albums.mockReturnValue({ data: { albums: [{ name: "Holidays", photoCount: 3 }] } });
  photos.mockReturnValue({ data: { photos: PHOTOS, nextCursor: "" }, isLoading: false, isFetching: false });
});

describe("the photo grid", () => {
  it("asks the server for a display-sized tile, with the token the browser needs", () => {
    render(<PhotosScreen />);
    const src = (document.querySelector(".card img") as HTMLImageElement).src;
    expect(src).toContain("/api/v1/photos/p-1/serve");
    expect(src).toContain("w=400");
    expect(src).toContain("token=tok");
  });

  it("opens the slideshow on the picture that was chosen", () => {
    render(<PhotosScreen />);
    fireEvent.click(document.querySelectorAll(".card")[2]);
    expect(navigateMock).toHaveBeenCalledWith(expect.objectContaining({ name: "slideshow", index: 2 }));
  });

  it("narrows to an album without leaving the library", () => {
    render(<PhotosScreen />);
    fireEvent.click(screen.getByText("Holidays (3)"));
    expect(photos).toHaveBeenLastCalledWith(expect.objectContaining({ album: "Holidays" }), expect.anything());
  });
});

describe("the slideshow", () => {
  // A key press holds at both ends; only the autoplay loops. Same rules as
  // the Android television (@evey/client's photoSlideshow).
  it("walks with left and right, holding at both ends", () => {
    render(<SlideshowScreen libraryId="lib-photos" index={0} />);
    expect(screen.getByText("1 / 3")).toBeTruthy();
    fireEvent.keyDown(window, { key: "ArrowLeft" });
    expect(screen.getByText("1 / 3")).toBeTruthy();
    fireEvent.keyDown(window, { key: "ArrowRight" });
    fireEvent.keyDown(window, { key: "ArrowRight" });
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(screen.getByText("3 / 3")).toBeTruthy();
  });

  it("holds and resumes on OK", () => {
    render(<SlideshowScreen libraryId="lib-photos" index={0} />);
    fireEvent.keyDown(window, { key: "Enter" });
    expect(screen.getByText((t) => t.includes("common.pause"))).toBeTruthy();
    fireEvent.keyDown(window, { key: "Enter" });
    expect(screen.queryByText((t) => t.includes("common.pause"))).toBeNull();
  });

  it("advances on its own and loops at the end of a fully loaded album", () => {
    vi.useFakeTimers();
    try {
      render(<SlideshowScreen libraryId="lib-photos" index={0} />);
      act(() => { vi.advanceTimersByTime(6000); });
      expect(screen.getByText("2 / 3")).toBeTruthy();
      act(() => { vi.advanceTimersByTime(12000); });
      expect(screen.getByText("1 / 3")).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

});

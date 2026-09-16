import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  TouchableOpacity: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { "aria-label": placeholder, placeholder, value: value ?? "", onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value) }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, f?: unknown) => (f && typeof f === "object" ? `${k}(${Object.values(f as object).join(",")})` : typeof f === "string" ? f : k),
    i18n: { language: "fr" },
  }),
}));
vi.mock("../BottomSheet", () => ({ BottomSheet: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null) }));
vi.mock("../GenreDot", () => ({ GenreDot: () => null }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { accent: "#f00", surfaceRaised: "#222", onAccent: "#000", text: "#fff", textMuted: "#888", border: "#333", bg: "#000" }, radii: { sm: 4, md: 8 } }),
}));

import { LibraryFiltersPanel, type LibraryFiltersPanelProps } from "./LibraryFiltersPanel";

function mount(over: Partial<LibraryFiltersPanelProps> = {}) {
  const h = {
    onClose: vi.fn(), onSortFieldChange: vi.fn(), onSortOrderChange: vi.fn(), onTypeFilterChange: vi.fn(), onGenreTextChange: vi.fn(), onGenresChange: vi.fn(),
    onYearMinChange: vi.fn(), onYearMaxChange: vi.fn(), onMinRatingChange: vi.fn(), onResolutionsChange: vi.fn(), onCodecsChange: vi.fn(), onAudioLangsChange: vi.fn(), onSubLangsChange: vi.fn(), onReset: vi.fn(),
  };
  render(
    <LibraryFiltersPanel
      visible
      sortField="MEDIA_SORT_FIELD_TITLE"
      sortOrder="SORT_ORDER_ASC"
      isShowLibrary={false}
      typeFilter=""
      genreText=""
      genres={[]}
      availableGenres={["Drame", "Action"]}
      yearMin=""
      yearMax=""
      resolutions={[]}
      codecs={["hevc"]}
      audioLangs={[]}
      subLangs={[]}
      availableFacets={{ resolutions: ["2160p"], codecs: ["hevc", "h264"], audioLangs: ["en", "zz"], subLangs: [] }}
      activeCount={0}
      {...h}
      {...over}
    />,
  );
  return h;
}

beforeEach(() => vi.clearAllMocks());

// Chips toggle in and out of their list without touching the rest, language
// chips read as language names with a bare code left for the unknown ones
// (Hermes ships no Intl data, so the table answers), the rating stars toggle
// off when tapped again, and the type row appears only on a show library.
describe("LibraryFiltersPanel", () => {
  it("toggles multi-select chips in and out and names languages", () => {
    const h = mount();
    fireEvent.click(screen.getByText("Drame"));
    expect(h.onGenresChange).toHaveBeenCalledWith(["Drame"]);
    fireEvent.click(screen.getByText("h264"));
    expect(h.onCodecsChange).toHaveBeenCalledWith(["hevc", "h264"]);
    fireEvent.click(screen.getByText("hevc"));
    expect(h.onCodecsChange).toHaveBeenLastCalledWith([]);
    fireEvent.click(screen.getByText("2160p"));
    expect(h.onResolutionsChange).toHaveBeenCalledWith(["2160p"]);
    const english = screen.getByText((txt) => txt.toLowerCase() === "english" || txt === "anglais");
    fireEvent.click(english);
    expect(h.onAudioLangsChange).toHaveBeenCalledWith(["en"]);
    expect(screen.getByText("zz")).toBeTruthy();
    expect(screen.queryByText("media.library.subtitleLanguage")).toBeNull();
  });

  it("stars set and clear the rating floor, sort and year fields report their values", () => {
    const h = mount({ minRating: 3 });
    fireEvent.click(screen.getByLabelText("media.library.minStars(3)"));
    expect(h.onMinRatingChange).toHaveBeenCalledWith(undefined);
    fireEvent.click(screen.getByLabelText("media.library.minStars(5)"));
    expect(h.onMinRatingChange).toHaveBeenCalledWith(5);
    fireEvent.click(screen.getByText("media.library.clearRating"));
    expect(h.onMinRatingChange).toHaveBeenLastCalledWith(undefined);
    fireEvent.change(screen.getByLabelText("media.search.fromYear"), { target: { value: "1999" } });
    expect(h.onYearMinChange).toHaveBeenCalledWith("1999");
    fireEvent.click(screen.getByText("↓ media.library.descending"));
    expect(h.onSortOrderChange).toHaveBeenCalledWith("SORT_ORDER_DESC");
    fireEvent.click(screen.getByText("media.library.year"));
    expect(h.onSortFieldChange).toHaveBeenCalledWith("MEDIA_SORT_FIELD_YEAR");
  });

  it("offers the type row on a show library only, and reset clears everything", () => {
    mount();
    expect(screen.queryByText("Type")).toBeNull();
    const h = mount({ isShowLibrary: true, activeCount: 3 });
    fireEvent.click(screen.getByText("media.library.episodes"));
    expect(h.onTypeFilterChange).toHaveBeenCalledWith("MEDIA_ITEM_TYPE_EPISODE");
    fireEvent.click(screen.getByText("media.library.clearAll (3)"));
    expect(h.onReset).toHaveBeenCalled();
  });
});

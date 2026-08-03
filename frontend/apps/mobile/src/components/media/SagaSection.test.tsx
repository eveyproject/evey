import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

const pushMock = vi.fn();
vi.mock("expo-router", () => ({ router: { push: (...a: unknown[]) => pushMock(...a) } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: { count?: number }) =>
      key === "media.person.titleCount" ? `${opts?.count} titles` : key,
  }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", surface: "#111" },
    radii: { md: 8 },
    spacing: { sm: 8, md: 12, lg: 16 },
  }),
}));

const useMediaItemCollectionsMock = vi.fn();
vi.mock("../../api/collections", () => ({
  useMediaItemCollections: (id: string) => useMediaItemCollectionsMock(id),
}));

import { SagaSection } from "./SagaSection";

describe("SagaSection", () => {
  it("renders each saga with its name and title count", () => {
    useMediaItemCollectionsMock.mockReturnValue({
      data: {
        collections: [
          { id: "c1", name: "Alien", itemCount: 4 },
          { id: "c2", name: "No Count Saga", itemCount: 0 },
        ],
      },
    });

    render(<SagaSection mediaItemId="m1" />);

    expect(screen.getByText("media.collections.partOf")).toBeTruthy();
    expect(screen.getByText("Alien")).toBeTruthy();
    expect(screen.getByText("4 titles")).toBeTruthy();
    expect(screen.getByText("No Count Saga")).toBeTruthy();
  });

  it("navigates to the collection page on press", () => {
    useMediaItemCollectionsMock.mockReturnValue({
      data: { collections: [{ id: "c1", name: "Alien", itemCount: 4 }] },
    });

    render(<SagaSection mediaItemId="m1" />);
    fireEvent.click(screen.getByLabelText("Alien"));
    expect(pushMock).toHaveBeenCalledWith("/collections/c1");
  });

  it("renders nothing when the title belongs to no collection", () => {
    useMediaItemCollectionsMock.mockReturnValue({ data: { collections: [] } });

    const { container } = render(<SagaSection mediaItemId="m1" />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("media.collections.partOf")).toBeNull();
  });

  it("renders nothing while the data has not loaded yet", () => {
    useMediaItemCollectionsMock.mockReturnValue({ data: undefined });

    const { container } = render(<SagaSection mediaItemId="m1" />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("media.collections.partOf")).toBeNull();
  });
});

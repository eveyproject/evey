import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", { "data-testid": "placeholder" }, children),
  StyleSheet: { create: (s: any) => s },
  PixelRatio: { get: () => 3 },
}));
vi.mock("expo-image", () => ({
  Image: ({ source, onError }: any) =>
    React.createElement("img", { "data-testid": "art", src: source.uri, "data-auth": source.headers?.Authorization, onError }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-testid": "icon" }, name) }));

import { TrackArt } from "./TrackArt";

const theme = { radii: { sm: 4 }, colors: { surfaceRaised: "#222", textMuted: "#888" } } as any;

// The cover is requested at the device's pixel density (a 40 dp tile on a 3x
// screen wants 120 px) with the auth header, and a missing or broken cover
// degrades to the notes placeholder instead of a blank tile - as does a
// missing server URL, which cannot even form the request.
describe("TrackArt", () => {
  it("asks the server for a density-scaled cover with the auth header", () => {
    render(<TrackArt trackID="t1" size={40} serverUrl="https://s" authHeaders={{ Authorization: "Bearer x" }} theme={theme} />);
    const img = screen.getByTestId("art");
    expect(img.getAttribute("src")).toBe("https://s/api/v1/music/tracks/t1/art?w=120");
    expect(img.getAttribute("data-auth")).toBe("Bearer x");
    expect(screen.queryByTestId("placeholder")).toBeNull();
  });

  it("falls back to the placeholder once the image errors", () => {
    render(<TrackArt trackID="t1" size={40} serverUrl="https://s" authHeaders={undefined} theme={theme} />);
    fireEvent.error(screen.getByTestId("art"));
    expect(screen.queryByTestId("art")).toBeNull();
    expect(screen.getByTestId("icon").textContent).toBe("musical-notes");
  });

  it("shows the placeholder without a server URL", () => {
    render(<TrackArt trackID="t1" size={40} serverUrl={undefined} authHeaders={undefined} theme={theme} />);
    expect(screen.queryByTestId("art")).toBeNull();
    expect(screen.getByTestId("placeholder")).toBeTruthy();
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Pressable: ({ children, onPress, accessibilityRole, testID }: any) => React.createElement("button", { onClick: onPress, role: accessibilityRole, "data-testid": testID }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { version?: string }) => (o?.version ? `${k}:${o.version}` : k) }),
}));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#fa0", text: "#fff" } }) }));
const route = vi.hoisted(() => ({ pathname: "/" }));
vi.mock("expo-router", () => ({ usePathname: () => route.pathname }));
const update = vi.hoisted(() => vi.fn());
vi.mock("../hooks/useAppUpdate", () => ({ useAppUpdate: () => update() }));
const openStorePage = vi.hoisted(() => vi.fn());
vi.mock("../lib/appUpdate", () => ({ openStorePage: () => openStorePage() }));

import { UpdateBanner } from "./UpdateBanner";

beforeEach(() => {
  vi.clearAllMocks();
  route.pathname = "/";
  update.mockReturnValue(null);
});

// The server names the phone build it was released with; this strip is the
// only way a viewer learns the store has not updated the app on its own.
describe("UpdateBanner", () => {
  it("renders nothing when this build is current", () => {
    render(<UpdateBanner />);
    expect(screen.queryByTestId("update-banner")).toBeNull();
  });

  it("names the newer build and opens its store page on tap", () => {
    update.mockReturnValue({ version: "0.0.1-beta.79", versionCode: 85 });
    render(<UpdateBanner />);
    fireEvent.click(screen.getByText("mobile.appUpdate.available:0.0.1-beta.79"));
    expect(openStorePage).toHaveBeenCalledTimes(1);
  });

  it("stays off the player", () => {
    update.mockReturnValue({ version: "0.0.1-beta.79", versionCode: 85 });
    route.pathname = "/player/abc";
    render(<UpdateBanner />);
    expect(screen.queryByTestId("update-banner")).toBeNull();
  });
});

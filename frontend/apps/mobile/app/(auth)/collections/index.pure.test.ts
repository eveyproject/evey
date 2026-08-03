import { describe, it, expect } from "vitest";
import React from "react";
import { vi } from "vitest";

// Pure-function tests only (no rendering) - these mocks exist solely to let
// the module import succeed outside a real React Native runtime; none of
// them are exercised since we never render the default-exported screen.
vi.mock("react-native", () => ({
  View: (p: any) => React.createElement("div", p),
  Text: (p: any) => React.createElement("span", p),
  FlatList: () => null,
  TouchableOpacity: (p: any) => React.createElement("button", p),
  StyleSheet: { create: (s: any) => s },
  Dimensions: { get: () => ({ width: 400, height: 800 }) },
  Alert: { alert: vi.fn() },
  Modal: () => null,
  TextInput: () => null,
  ActivityIndicator: () => null,
  KeyboardAvoidingView: (p: any) => React.createElement("div", p),
  Platform: { OS: "ios" },
}));
vi.mock("expo-image", () => ({ Image: () => null }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("../../../src/components/Skeleton", () => ({ Skeleton: () => null }));
vi.mock("../../../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: {}, radii: {} }) }));
vi.mock("../../../src/api/collections", () => ({
  useCollections: vi.fn(),
  useCreateCollection: vi.fn(),
  useDeleteCollection: vi.fn(),
}));

import { collectionSubtitle } from "./index";

describe("collectionSubtitle (mobile)", () => {
  const t = (key: string, options?: Record<string, unknown>) =>
    options?.count !== undefined ? `${options.count} titles` : key;

  it("returns no parts for a manual collection with no known count", () => {
    expect(collectionSubtitle(undefined, undefined, t)).toEqual({});
  });

  it("returns a badge for a saga collection", () => {
    expect(collectionSubtitle("saga", undefined, t)).toEqual({ badge: "media.collections.saga" });
  });

  it("returns a count when itemCount is positive, regardless of kind", () => {
    expect(collectionSubtitle("manual", 7, t)).toEqual({ count: "7 titles" });
  });

  it("omits the count when itemCount is zero", () => {
    expect(collectionSubtitle(undefined, 0, t)).toEqual({});
  });

  it("combines badge and count for a saga with visible titles", () => {
    expect(collectionSubtitle("saga", 3, t)).toEqual({ badge: "media.collections.saga", count: "3 titles" });
  });
});

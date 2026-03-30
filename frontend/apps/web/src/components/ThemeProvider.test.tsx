import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ThemeProvider, isPluginTheme, pluginNameFromTheme, pluginThemeName } from "./ThemeProvider";

let mockTheme = "default";
let mockPluginId: string | null = null;
const mockApplyThemeToDOM = vi.fn();
const mockUnloadAllPluginThemes = vi.fn();
const mockLoadPluginTheme = vi.fn();

vi.mock("@evey/store", () => ({
  useThemeStore: () => ({ theme: mockTheme, pluginId: mockPluginId, customVars: undefined }),
}));

vi.mock("@evey/theme", () => ({
  themes: { default: {}, amoled: {} },
  applyThemeToDOM: (...args: unknown[]) => mockApplyThemeToDOM(...args),
}));

vi.mock("@/features/plugins/theme-loader", () => ({
  loadPluginTheme: (...args: unknown[]) => mockLoadPluginTheme(...args),
  unloadAllPluginThemes: () => mockUnloadAllPluginThemes(),
}));

describe("ThemeProvider", () => {
  beforeEach(() => {
    mockTheme = "default";
    mockPluginId = null;
    vi.clearAllMocks();
    document.documentElement.removeAttribute("data-theme");
  });

  it("renders children", () => {
    render(<ThemeProvider><span>Hello</span></ThemeProvider>);
    expect(screen.getByText("Hello")).toBeInTheDocument();
  });

  it("applies built-in theme via applyThemeToDOM", () => {
    mockTheme = "amoled";
    render(<ThemeProvider><span>child</span></ThemeProvider>);
    expect(mockApplyThemeToDOM).toHaveBeenCalledWith("amoled", undefined);
    expect(mockUnloadAllPluginThemes).toHaveBeenCalled();
  });

  it("applies plugin theme with data-theme attribute", () => {
    mockTheme = "plugin:my-theme";
    mockPluginId = "plugin-123";
    render(<ThemeProvider><span>child</span></ThemeProvider>);
    expect(document.documentElement.getAttribute("data-theme")).toBe("my-theme");
    expect(mockLoadPluginTheme).toHaveBeenCalledWith("plugin-123");
  });
});

describe("isPluginTheme", () => {
  it("returns true for plugin: prefix", () => {
    expect(isPluginTheme("plugin:neon")).toBe(true);
  });

  it("returns false for built-in theme", () => {
    expect(isPluginTheme("default")).toBe(false);
  });
});

describe("pluginNameFromTheme", () => {
  it("strips plugin: prefix", () => {
    expect(pluginNameFromTheme("plugin:neon")).toBe("neon");
  });
});

describe("pluginThemeName", () => {
  it("adds plugin: prefix", () => {
    expect(pluginThemeName("neon")).toBe("plugin:neon");
  });
});

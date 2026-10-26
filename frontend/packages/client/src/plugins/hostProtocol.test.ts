import { describe, it, expect, vi } from "vitest";
import {
  buildConnectSrc,
  frameLoadMessage,
  isFrameShellMessage,
  pluginFrameUrl,
  buildInitPayload,
  clampSlotHeight,
  createToastLimiter,
  inlineScriptSafe,
  isAllowedNavPath,
  isLightBackground,
  isValidPluginId,
  parseHostMessage,
} from "./hostProtocol";
import { entryPathFor, resolveSlotEntries } from "./slotEntries";
import { isValidAssetPath, pluginRequestFor } from "./pluginRpc";

// The rules every host must apply the same way. They used to live three
// times (iframe, phone WebView, TV WebView); a rule that drifted on one host
// was a plugin able to do on one screen what it could not on another.

describe("isAllowedNavPath", () => {
  it("accepts the non-sensitive prefixes and their sub-paths", () => {
    for (const p of ["/home", "/library/abc", "/media/m1?tab=cast", "/player/x", "/settings", "/search?q=a"]) {
      expect(isAllowedNavPath(p), p).toBe(true);
    }
  });
  it("refuses admin, auth, absolute and protocol-relative targets", () => {
    for (const p of ["/admin", "/admin/plugins", "/auth/login", "/setup", "/profile/security", "//evil.com", "https://evil.com", "javascript:alert(1)", "/homeless", ""]) {
      expect(isAllowedNavPath(p), p).toBe(false);
    }
  });
  it("refuses a path longer than the cap", () => {
    expect(isAllowedNavPath("/home/" + "a".repeat(2048))).toBe(false);
  });
});

describe("isValidPluginId", () => {
  it("takes the server's plg- slugs and dev slugs, refuses HTML-special characters", () => {
    expect(isValidPluginId("plg-0123456789abcdef01234567")).toBe(true);
    expect(isValidPluginId("plg-weather_v2")).toBe(true);
    expect(isValidPluginId("weather")).toBe(false);
    expect(isValidPluginId('plg-x"><script>')).toBe(false);
    expect(isValidPluginId("plg-a/b")).toBe(false);
    expect(isValidPluginId("plg-" + "a".repeat(65))).toBe(false);
  });
});

describe("document helpers", () => {
  it("prefixes bare hosts with https and answers 'none' for an empty allow-list", () => {
    expect(buildConnectSrc([])).toBe("'none'");
    expect(buildConnectSrc(["api.open-meteo.com", "http://localhost:8080"])).toBe("https://api.open-meteo.com http://localhost:8080");
  });
  it("drops an entry that is not a plain host, since the list is written into a CSP meta tag", () => {
    expect(
      buildConnectSrc([
        "api.open-meteo.com",
        "evil.example.com; script-src 'unsafe-inline'",
        'x.example.com"><script>alert(1)</script>',
        "*",
        "ftp://files.example.com",
        "10.0.0.1",
      ]),
    ).toBe("https://api.open-meteo.com");
    expect(buildConnectSrc(["a.example.com'", " "])).toBe("'none'");
  });
  it("defuses a literal </script> in a bundle", () => {
    expect(inlineScriptSafe('x="</script><b>"; y="</SCRIPT>"')).toBe('x="<\\/script><b>"; y="<\\/script>"');
  });
  it("reads a light theme off a bright background only", () => {
    expect(isLightBackground("#ffffff")).toBe(true);
    expect(isLightBackground("#0a0a0a")).toBe(false);
    expect(isLightBackground("#fff")).toBe(false);
  });
});

describe("clampSlotHeight", () => {
  it("keeps 0 (collapse), floors small values, caps large ones", () => {
    expect(clampSlotHeight(0, 50)).toBe(0);
    expect(clampSlotHeight(10, 50)).toBe(50);
    expect(clampSlotHeight(300, 50)).toBe(300);
    expect(clampSlotHeight(5000, 60)).toBe(800);
  });
});

describe("createToastLimiter", () => {
  it("lets three toasts through per second, drops the rest, truncates the text", () => {
    let now = 1000;
    const sink = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
    const limited = createToastLimiter(sink, () => now);
    expect(limited("success", "a")).toBe(true);
    expect(limited("error", "b")).toBe(true);
    expect(limited("weird", "c".repeat(300))).toBe(true);
    expect(limited("info", "d")).toBe(false);
    now += 1001;
    expect(limited("info", "e")).toBe(true);
    expect(sink.success).toHaveBeenCalledWith("a");
    expect(sink.error).toHaveBeenCalledWith("b");
    expect(sink.info).toHaveBeenCalledWith("c".repeat(200));
    expect(sink.info).toHaveBeenCalledWith("e");
  });
});

describe("parseHostMessage", () => {
  it("reads objects and JSON strings, refuses anything half-shaped", () => {
    expect(parseHostMessage({ type: "ready" })).toEqual({ type: "ready" });
    expect(parseHostMessage(JSON.stringify({ type: "navigate", path: "/home" }))).toEqual({ type: "navigate", path: "/home" });
    expect(parseHostMessage({ type: "navigate" })).toBeNull();
    expect(parseHostMessage({ type: "resize", height: -1 })).toBeNull();
    expect(parseHostMessage({ type: "resize", height: 0 })).toEqual({ type: "resize", height: 0 });
    expect(parseHostMessage({ type: "rpc", id: 3, method: "prefs.get", args: { key: "k" } })).toEqual({ type: "rpc", id: 3, method: "prefs.get", args: { key: "k" } });
    expect(parseHostMessage({ type: "rpc", id: 3 })).toEqual({ type: "rpc", id: 3, method: "", args: {} });
    expect(parseHostMessage("not json{")).toBeNull();
    expect(parseHostMessage(null)).toBeNull();
    expect(parseHostMessage({ type: "steal" })).toBeNull();
  });
});

describe("buildInitPayload", () => {
  it("omits the optional heights when the surface has none", () => {
    expect(buildInitPayload({ context: undefined, config: { a: 1 }, locale: "fr", theme: "dark", surface: "web" })).toEqual({
      type: "init", context: {}, config: { a: 1 }, locale: "fr", theme: "dark", surface: "web",
    });
    expect(buildInitPayload({ context: { m: 1 }, config: {}, locale: "en", theme: "light", surface: "tv", suggestedHeight: 173, viewportHeight: 1080 })).toMatchObject({
      context: { m: 1 }, surface: "tv", suggestedHeight: 173, viewportHeight: 1080,
    });
  });
});

describe("slot entries", () => {
  it("strips the web_root prefix from a module src", () => {
    expect(entryPathFor("dist/index.js", "dist")).toBe("index.js");
    expect(entryPathFor("dist/index.js", "dist/")).toBe("index.js");
    expect(entryPathFor("dist", "dist")).toBe("");
    expect(entryPathFor("index.js", ".")).toBe("index.js");
    expect(entryPathFor("index.js", "")).toBe("index.js");
  });
  it("lists the enabled plugins mounting the slot, both casings of the manifest fields, with the filters", () => {
    const plugins = [
      { id: "plg-a", name: "A", frontend: [{ slot: "home.section", src: "dist/a.js" }], web_root: "dist", external_fetch: ["x.io"] },
      { id: "plg-b", frontend: [{ slot: "home.section", src: "b.js" }, { slot: "admin.nav", src: "n.js" }], webRoot: ".", externalFetch: [] },
      { id: "plg-c", enabled: false, frontend: [{ slot: "home.section", src: "c.js" }] },
      { id: "", frontend: [{ slot: "home.section", src: "z.js" }] },
    ];
    expect(resolveSlotEntries(plugins, "home.section")).toEqual([
      { pluginId: "plg-a", name: "A", entryPath: "a.js", externalFetch: ["x.io"] },
      { pluginId: "plg-b", name: "plg-b", entryPath: "b.js", externalFetch: [] },
    ]);
    expect(resolveSlotEntries(plugins, "home.section", { pluginId: "plg-b" }).map((e) => e.pluginId)).toEqual(["plg-b"]);
    expect(resolveSlotEntries(plugins, "home.section", { excludeRegistered: new Set(["plg-b"]) }).map((e) => e.pluginId)).toEqual(["plg-a"]);
    expect(resolveSlotEntries(undefined, "home.section")).toEqual([]);
  });
});

describe("pluginRequestFor", () => {
  it("maps the allowlisted methods and nothing else", () => {
    expect(pluginRequestFor("plg-a", "prefs.get", { key: "a b" }, "T")).toEqual({ path: "/api/v1/plugin-prefs/a%20b", method: "GET", authToken: "T" });
    expect(pluginRequestFor("plg-a", "prefs.set", { key: "k", value: 1 }, "T")).toMatchObject({ method: "PUT", body: JSON.stringify({ value: "1" }) });
    expect(pluginRequestFor("plg-a", "media.list", { libraryId: "L" }, "T")).toMatchObject({ path: "/api/v1/public/media?libraryId=L" });
    // The poster goes out on the USER's session: the artwork handler applies
    // this user's library ACL and parental filter.
    expect(pluginRequestFor("plg-a", "media.poster", { id: "m1" }, "T")).toEqual({ path: "/api/v1/media/m1/artwork/poster", method: "GET" });
    expect(pluginRequestFor("plg-a", "assets.read", { path: "img/a.png" }, "T")?.path).toMatch(/^\/api\/v1\/plugins\/plg-a\/assets\/img\/a\.png\?_=\d+$/);
    expect(pluginRequestFor("plg-a", "assets.read", { path: "../secret" }, "T")).toBeNull();
    expect(pluginRequestFor("plg-a", "fs.read", {}, "T")).toBeNull();
  });
  it("guards asset paths against traversal and odd characters", () => {
    expect(isValidAssetPath("a/b.png")).toBe(true);
    for (const p of ["", "/etc", "a/../b", "a\\b", "a b", "a".repeat(257)]) expect(isValidAssetPath(p), p).toBe(false);
  });
});

// The frame page is Evey's own, the hosts ride as a hint the server validates,
// and nothing secret is ever part of the URL.
describe("plugin frame page", () => {
  it("builds the frame URL from the manifest's hosts", () => {
    expect(pluginFrameUrl([])).toBe("/plugin-frame.html");
    expect(pluginFrameUrl([" api.open-meteo.com ", "", "http://localhost:8080"])).toBe(
      "/plugin-frame.html?connect=" + encodeURIComponent("api.open-meteo.com,http://localhost:8080"),
    );
  });

  it("recognises the handshake and shapes the load message", () => {
    expect(isFrameShellMessage({ type: "evey-frame-shell" })).toBe(true);
    expect(isFrameShellMessage({ type: "ready" })).toBe(false);
    expect(isFrameShellMessage(null)).toBe(false);
    expect(frameLoadMessage(["a", "b"])).toEqual({ type: "evey-frame-load", scripts: ["a", "b"] });
    expect(frameLoadMessage(["a"], ":root{}", "https://h/x/")).toEqual({ type: "evey-frame-load", scripts: ["a"], css: ":root{}", base: "https://h/x/" });
  });
});

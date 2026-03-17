import { describe, it, expect, beforeEach } from "vitest";
import { getPluginsForSlot, clearRegistry, loadPlugin } from "./plugin-registry";

beforeEach(() => {
  clearRegistry();
});

describe("plugin-registry", () => {
  it("returns empty for unknown slot", () => {
    expect(getPluginsForSlot("nonexistent")).toEqual([]);
  });

  it("loadPlugin stores and retrieves by slot", async () => {
    const fakeUrl = "data:text/javascript," + encodeURIComponent("export default function TestWidget() { return null; }");
    await loadPlugin("p1", "player.overlay", fakeUrl);

    const widgets = getPluginsForSlot("player.overlay");
    expect(widgets).toHaveLength(1);
    expect(widgets[0].pluginId).toBe("p1");
    expect(typeof widgets[0].component).toBe("function");
  });

  it("deduplicates same plugin+slot", async () => {
    const fakeUrl = "data:text/javascript," + encodeURIComponent("export default function W() { return null; }");
    await loadPlugin("p1", "home.section", fakeUrl);
    await loadPlugin("p1", "home.section", fakeUrl);

    expect(getPluginsForSlot("home.section")).toHaveLength(1);
  });

  it("clearRegistry removes all", async () => {
    const fakeUrl = "data:text/javascript," + encodeURIComponent("export default function W() { return null; }");
    await loadPlugin("p1", "sidebar.bottom", fakeUrl);
    clearRegistry();
    expect(getPluginsForSlot("sidebar.bottom")).toEqual([]);
  });
});

import { describe, it, expect } from "vitest";
import { installPreviewHasWarnings, type InstallPreview } from "./pluginConfigUtils";

// The install-from-URL modal previews the source before installing (it used
// to install blind). The confirm button must read "Install anyway" exactly
// when the web page would say so.
describe("installPreviewHasWarnings", () => {
  const clean: InstallPreview = { name: "p", version: "1.0.0", permissions: ["api.library"], analysis: { safe: true, issues: [] } };

  it("a clean preview installs plainly", () => {
    expect(installPreviewHasWarnings(clean)).toBe(false);
    expect(installPreviewHasWarnings({ name: "p", version: "1" })).toBe(false);
  });

  it("an analysis WARNING is a warning even when the report says safe", () => {
    expect(installPreviewHasWarnings({ ...clean, analysis: { safe: true, issues: ["[WARNING] overreach: docker invoked at runtime"] } })).toBe(true);
  });

  it("a subprocess permission is a warning on its own", () => {
    expect(installPreviewHasWarnings({ ...clean, permissions: ["subprocess"] })).toBe(true);
  });
});

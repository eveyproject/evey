import { createRequire } from "node:module";
import { describe, it, expect } from "vitest";

// The window background is dropped in MainActivity by a config plugin, since
// apps/tv/android is a prebuild artifact git ignores. Pin where the call lands:
// after super.onCreate (the theme background is the launch screen until then),
// once even when prebuild runs twice.
const require_ = createRequire(import.meta.url);
const { applyNoWindowBackground } = require_("../../plugins/withNoWindowBackground.js") as {
  applyNoWindowBackground: (src: string) => string;
};

const MAIN_ACTIVITY = `class MainActivity : ReactActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    setTheme(R.style.AppTheme);
    super.onCreate(null)
    (application as? MainApplication)?.applyUiScale()
  }
}
`;

describe("withNoWindowBackground", () => {
  it("drops the window background right after super.onCreate, once", () => {
    const out = applyNoWindowBackground(MAIN_ACTIVITY);
    expect(out).toContain("    super.onCreate(null)\n    window.setBackgroundDrawable(null)");
    expect(applyNoWindowBackground(out)).toBe(out);
    expect(out.split("setBackgroundDrawable").length).toBe(2);
  });

  it("refuses a MainActivity it does not recognise instead of silently doing nothing", () => {
    expect(() => applyNoWindowBackground("class MainActivity {}")).toThrow(/super\.onCreate/);
  });
});

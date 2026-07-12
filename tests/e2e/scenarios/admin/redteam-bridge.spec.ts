import { test, expect } from "../../fixtures/auth";
import { goto } from "../shared/helpers";

// Reads the redteam-bridge plugin's in-iframe probe results from the home page.
// The plugin is installed in dev (plg id below); it tries to bypass the iframe
// sandbox/CSP and the prefs/public scope boundaries and renders a verdict each.
// Opt-in dev verification: install tests/plugins/redteam-bridge, then run with
// RT_PLUGIN_ID=<plugin id>. Skipped otherwise (the plugin isn't part of CI seed).
const PLUGIN_ID = process.env.RT_PLUGIN_ID;

test("redteam-bridge: security layers hold from inside the iframe", async ({ page }) => {
  test.skip(!PLUGIN_ID, "set RT_PLUGIN_ID to the installed redteam-bridge plugin id");
  await goto(page, "/home");
  const frame = page.frameLocator(`iframe[title*="${PLUGIN_ID}"]`);
  const results = frame.getByTestId("rt-results");
  await expect(results).toBeVisible({ timeout: 20_000 });

  // Wait for all async probes to land.
  await expect(results).toContainText("media_filepath_leak=", { timeout: 15_000 });
  const text = (await results.textContent()) ?? "";
  // eslint-disable-next-line no-console
  console.log("RED-TEAM RESULTS:\n" + text.replace(/=/g, " = ").replace(/\)/g, ")\n"));

  // Direct API access from the iframe is blocked by CSP (connect-src is the
  // plugin's external_fetch list - empty here).
  expect(text).toContain("direct_admin_fetch=BLOCKED");
  expect(text).toContain("direct_prefs_fetch=BLOCKED");
  // The sanctioned, granted path works (own namespace).
  expect(text).toContain("bridge_prefs_roundtrip=OK");
  // The public media projection never leaks a file path.
  expect(text).toContain("media_filepath_leak=BLOCKED");
});

// E2E security gate: every malicious plugin fixture must be rejected by the
// server's static analyzer when uploaded via the admin REST endpoint.
// The upload endpoint (/api/v1/admin/plugins/upload) is the same one the UI
// calls via eveyFetchRaw after the user clicks "Preview & Install" → "Install".
//
// Auth: relies on EVEY_USER / EVEY_PASS env vars via global-setup.
import { test, expect, request as playwrightRequest } from "@playwright/test";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import {
  PLUGIN_ZIP_DIR,
  getAccessToken,
  uploadPluginZip,
  assertUploadBlocked,
  assertBlockedWithCategory,
} from "../../helpers/plugin-helpers";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const STORAGE_STATE = path.join(__dirname, "../../.auth-state.json");

test.use({ storageState: STORAGE_STATE, ignoreHTTPSErrors: true });

// Fixtures mapped to the analyzer category they must trigger (CRITICAL).
// Source of truth: tests/fixtures/plugins/malicious/README.md and
// internal/app/plugin/installer_security_test.go.
const MALICIOUS_FIXTURES: Array<{ name: string; expectedCategory: string }> = [
  // "sys" . "tem" string-concat fold → system() after normalization
  { name: "php-string-concat-rce",   expectedCategory: "shell_exec" },
  // outer eval( is plaintext, inner f-string fragment is decoded at normalize
  { name: "python-fstring-eval",     expectedCategory: "obfuscation" },
  // Function.constructor escapes Node vm sandbox
  { name: "js-constructor-escape",   expectedCategory: "sandbox_escape" },
  // os aliased to _o then m.system() — alias pre-pass resolves to os.system
  { name: "python-os-alias",         expectedCategory: "shell_exec" },
  // getattr(__builtins__, "exec") bypasses static import restrictions
  { name: "python-getattr-builtin",  expectedCategory: "dynamic_import" },
  // syscall.Syscall(59, ...) = raw execve, no stdlib wrapper
  { name: "go-raw-syscall",          expectedCategory: "native_code" },
  // Multi-vector: RCE + JWT exfil + outbound HTTP
  { name: "php-combo-realistic",     expectedCategory: "" },
];

test.describe("plugin security — install rejection of malicious fixtures", () => {
  for (const fixture of MALICIOUS_FIXTURES) {
    test(`refuses ${fixture.name}`, async ({ page }) => {
      const zipPath = path.join(PLUGIN_ZIP_DIR, `${fixture.name}.zip`);
      const token = await getAccessToken(page);

      // Use an isolated APIRequestContext so multipart upload doesn't require
      // the browser session cookie (same pattern as plugin-weather.spec.ts).
      const apiCtx = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
      try {
        const result = await uploadPluginZip(apiCtx, token, zipPath);

        // Primary assertion: server must return 403 blocked
        if (fixture.expectedCategory) {
          assertBlockedWithCategory(result, fixture.name, fixture.expectedCategory);
        } else {
          assertUploadBlocked(result, fixture.name);
        }

        // The plugin must NOT appear in the installed list
        await page.goto("/admin/plugins", { waitUntil: "domcontentloaded", timeout: 10_000 });
        await page.waitForTimeout(1_500);
        const body = await page.locator("body").textContent();
        // The fixture slug should not appear as an installed plugin name
        expect(body ?? "").not.toContain(fixture.name + " ");
      } finally {
        await apiCtx.dispose();
      }
    });
  }
});

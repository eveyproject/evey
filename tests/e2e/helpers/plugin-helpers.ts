// Helpers for plugin security e2e tests.
// Upload goes via the admin API (multipart POST) — the same path the UI calls
// via eveyFetchRaw — so we validate the real security gate without having to
// drive the multi-step review modal through Playwright's file-picker.
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import type { Page, APIRequestContext } from "@playwright/test";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

/** Absolute path to the pre-built malicious plugin ZIPs. */
export const PLUGIN_ZIP_DIR = path.join(__dirname, "..", "fixtures", "plugin-zips");

const BASE_URL = process.env.BASE_URL ?? "https://evey.test";

/** Read the JWT the global-setup persisted in localStorage. */
export async function getAccessToken(page: Page): Promise<string> {
  if (page.url() === "about:blank" || !page.url().startsWith(BASE_URL)) {
    await page.goto(`${BASE_URL}/home`, { waitUntil: "domcontentloaded", timeout: 10_000 });
  }
  const tok = await page.evaluate(() => {
    const raw = localStorage.getItem("evey-auth");
    if (!raw) return null;
    try {
      const parsed = JSON.parse(raw) as { state?: { accessToken?: string } };
      return parsed.state?.accessToken ?? null;
    } catch {
      return null;
    }
  });
  if (!tok) throw new Error("no accessToken in localStorage — global-setup did not run?");
  return tok;
}

/** Upload a plugin ZIP via the admin REST endpoint (same endpoint as the UI). */
export async function uploadPluginZip(
  request: APIRequestContext,
  token: string,
  zipPath: string,
): Promise<{ status: number; body: Record<string, unknown> }> {
  if (!fs.existsSync(zipPath)) {
    throw new Error(`ZIP not found: ${zipPath} — run tests/e2e/fixtures/plugin-zips/build.sh first`);
  }
  const zipBytes = fs.readFileSync(zipPath);
  const fileName = path.basename(zipPath);
  const res = await request.post(`${BASE_URL}/api/v1/admin/plugins/upload`, {
    headers: { Authorization: `Bearer ${token}` },
    multipart: {
      plugin: { name: fileName, mimeType: "application/zip", buffer: zipBytes },
    },
  });
  let body: Record<string, unknown> = {};
  try {
    body = (await res.json()) as Record<string, unknown>;
  } catch {
    // non-JSON response — leave body empty
  }
  return { status: res.status(), body };
}

/**
 * Assert the upload was rejected with a security block.
 * The server returns HTTP 403 with { blocked: true, issues: [...] }.
 */
export function assertUploadBlocked(
  result: { status: number; body: Record<string, unknown> },
  fixtureLabel: string,
): void {
  if (result.status !== 403) {
    throw new Error(
      `[${fixtureLabel}] expected HTTP 403 (blocked), got ${result.status}. Body: ${JSON.stringify(result.body)}`,
    );
  }
  if (!result.body["blocked"]) {
    throw new Error(
      `[${fixtureLabel}] expected body.blocked=true, got: ${JSON.stringify(result.body)}`,
    );
  }
}

/**
 * Assert the upload was rejected AND the analyzer flagged the expected category.
 * The server returns a deduplicated `categories: string[]` machine-readable
 * enum list (e.g. ["shell_exec", "sandbox_escape", "native_code"]) alongside
 * the human-readable `issues` strings. We assert against `categories` —
 * matching enums is robust to wording changes in the issue descriptions.
 * Falls back to a substring scan of `issues` for older server builds that
 * don't emit `categories` yet (so the test stays useful across versions).
 */
export function assertBlockedWithCategory(
  result: { status: number; body: Record<string, unknown> },
  fixtureLabel: string,
  expectedCategory: string,
): void {
  assertUploadBlocked(result, fixtureLabel);
  const want = expectedCategory.toLowerCase();
  const categories = (result.body["categories"] as string[] | undefined) ?? [];
  if (categories.length > 0) {
    if (!categories.map((c) => c.toLowerCase()).includes(want)) {
      throw new Error(
        `[${fixtureLabel}] categories ${JSON.stringify(categories)} does not include "${expectedCategory}".`,
      );
    }
    return;
  }
  // Fallback for legacy server responses without the categories field.
  const issues = (result.body["issues"] as string[] | undefined) ?? [];
  const joined = issues.join("\n").toLowerCase();
  if (!joined.includes(want)) {
    throw new Error(
      `[${fixtureLabel}] no categories field and issues list does not mention "${expectedCategory}".\nIssues:\n${issues.join("\n")}`,
    );
  }
}

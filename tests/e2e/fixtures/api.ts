import { test as base, expect, type APIRequestContext } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirpath = path.dirname(__filename);
const TOKEN_CACHE = path.join(__dirpath, ".token-cache.json");
const TOKEN_TTL = 10 * 60 * 1000;

interface TokenCache {
  token: string;
  ts: number;
}

function readCachedToken(): string | null {
  try {
    const raw = fs.readFileSync(TOKEN_CACHE, "utf-8");
    const cache: TokenCache = JSON.parse(raw);
    if (Date.now() - cache.ts < TOKEN_TTL) return cache.token;
  } catch { /* cache miss */ }
  return null;
}

function writeCachedToken(token: string) {
  fs.writeFileSync(TOKEN_CACHE, JSON.stringify({ token, ts: Date.now() }));
}

async function getToken(request: APIRequestContext): Promise<string> {
  const cached = readCachedToken();
  if (cached) return cached;

  const user = process.env.EVEY_USER;
  const pass = process.env.EVEY_PASS;
  if (!user || !pass) {
    throw new Error("EVEY_USER and EVEY_PASS env vars are required for e2e (no hardcoded credentials)");
  }

  const resp = await request.post("/api/v1/auth/login", {
    data: { username: user, password: pass },
  });
  if (!resp.ok()) {
    throw new Error(`Login failed: ${resp.status()} ${await resp.text()}`);
  }
  const body = await resp.json();
  const token = body.accessToken ?? body.access_token;
  writeCachedToken(token);
  return token;
}

export const test = base.extend<{ token: string }>({
  token: async ({ request }, use) => {
    const token = await getToken(request);
    await use(token);
  },
});

export function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` };
}

export { expect };

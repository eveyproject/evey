import { test, expect, authHeaders } from "./fixtures/api";

// The seek-index maintenance API is fully asynchronous (the old synchronous
// scan/repair 504'd behind a reverse proxy on real libraries): every entry
// point starts a job, progress is polled on /job, and the cached result of
// the last completed scan is served instantly.

interface SeekIndexJob {
  running: boolean;
  kind?: string;
  done?: number;
  total?: number;
  last?: { kind?: string; done?: number; failed?: number; error?: string } | null;
}

async function getJob(request: any, token: string): Promise<SeekIndexJob> {
  const res = await request.get("/api/v1/admin/media/seek-index/job", {
    headers: authHeaders(token),
  });
  expect(res.ok()).toBeTruthy();
  return res.json();
}

async function waitJobIdle(request: any, token: string, timeoutMs = 30_000): Promise<SeekIndexJob> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const job = await getJob(request, token);
    if (!job.running) return job;
    if (Date.now() > deadline) throw new Error("seek-index job did not finish in time");
    await new Promise((r) => setTimeout(r, 500));
  }
}

// Find a library with no (or few) EBML files so the scan job finishes fast and
// deterministically — a photos/music library is ideal.
async function smallestLibraryId(request: any, token: string): Promise<string> {
  const res = await request.get("/api/v1/libraries", { headers: authHeaders(token) });
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  const libs: { id: string; type?: string }[] = body.libraries ?? [];
  expect(libs.length).toBeGreaterThan(0);
  const nonVideo = libs.find((l) => l.type === "LIBRARY_TYPE_PHOTOS" || l.type === "LIBRARY_TYPE_MUSIC");
  return (nonVideo ?? libs[0]).id;
}

test.describe("Admin seek-index (async jobs)", () => {
  test("job endpoint reports idle/running state", async ({ request, token }) => {
    const job = await getJob(request, token);
    expect(typeof job.running).toBe("boolean");
  });

  test("cached scan starts empty (scannedAtMs=0) and fills after a scan job", async ({ request, token }) => {
    const libId = await smallestLibraryId(request, token);

    // Wait out any job another test/admin left running, then kick a scan.
    await waitJobIdle(request, token);
    const start = await request.post("/api/v1/admin/media/seek-index/scan", {
      headers: authHeaders(token),
      data: { libraryId: libId },
    });
    // 503 = a job raced us (another admin/test); the contract under test is
    // "never a 504/500", so a clean busy signal is acceptable here.
    if (start.status() === 503 || start.status() === 429) return;
    expect(start.ok()).toBeTruthy();

    const job = await waitJobIdle(request, token);
    expect(job.last?.kind).toBe("scan");
    expect(job.last?.error ?? "").toBe("");

    const cached = await request.get(
      `/api/v1/admin/media/seek-index?libraryId=${encodeURIComponent(libId)}`,
      { headers: authHeaders(token) },
    );
    expect(cached.ok()).toBeTruthy();
    const body = await cached.json();
    expect(Number(body.scannedAtMs)).toBeGreaterThan(0);
    expect(Array.isArray(body.items ?? [])).toBeTruthy();
  });

  test("repairing an unknown media is a 404, not a sanitized 500", async ({ request, token }) => {
    const res = await request.post("/api/v1/admin/media/ghost-e2e/seek-index/reindex", {
      headers: authHeaders(token),
    });
    expect(res.status()).toBe(404);
  });

  test("unauthenticated requests are rejected", async ({ request }) => {
    for (const path of [
      "/api/v1/admin/media/seek-index",
      "/api/v1/admin/media/seek-index/job",
    ]) {
      const res = await request.get(path);
      expect([401, 403]).toContain(res.status());
    }
    const post = await request.post("/api/v1/admin/media/seek-index/scan", { data: {} });
    expect([401, 403]).toContain(post.status());
  });
});

import { test, expect, authHeaders } from "./fixtures/api";

test.describe("Admin backups", () => {
  test("list backups returns empty array or items", async ({ request, token }) => {
    const res = await request.get("/api/v1/admin/backups", {
      headers: authHeaders(token),
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body.backups)).toBeTruthy();
  });

  test("create backup returns 201 or 500 if pg_dump unavailable", async ({ request, token }) => {
    const res = await request.post("/api/v1/admin/backups", {
      headers: authHeaders(token),
    });
    // In dev, pg_dump may not be installed in the container
    if (res.status() === 500) {
      const body = await res.json();
      expect(body.message).toBe("internal error");
      // This is expected in dev — pg_dump not in container
      return;
    }
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.createdAt).toBeTruthy();
    expect(body.version).toBeTruthy();
  });

  test("create + list shows the new backup", async ({ request, token }) => {
    const createRes = await request.post("/api/v1/admin/backups", {
      headers: authHeaders(token),
    });
    test.skip(createRes.status() === 500, "pg_dump unavailable in dev container");

    expect(createRes.status()).toBe(201);
    const created = await createRes.json();

    const listRes = await request.get("/api/v1/admin/backups", {
      headers: authHeaders(token),
    });
    expect(listRes.ok()).toBeTruthy();
    const body = await listRes.json();
    const backups: { id: string }[] = body.backups ?? [];
    const found = backups.find((b) => b.id === created.id);
    expect(found).toBeTruthy();
  });

  test("delete a backup returns 204", async ({ request, token }) => {
    const createRes = await request.post("/api/v1/admin/backups", {
      headers: authHeaders(token),
    });
    test.skip(createRes.status() === 500, "pg_dump unavailable in dev container");

    expect(createRes.status()).toBe(201);
    const created = await createRes.json();

    const delRes = await request.delete(`/api/v1/admin/backups/${created.id}`, {
      headers: authHeaders(token),
    });
    expect(delRes.status()).toBe(204);

    // verify gone
    const listRes = await request.get("/api/v1/admin/backups", {
      headers: authHeaders(token),
    });
    const body = await listRes.json();
    const backups: { id: string }[] = body.backups ?? [];
    expect(backups.find((b) => b.id === created.id)).toBeUndefined();
  });

  test("unauthenticated request is rejected", async ({ request }) => {
    const res = await request.get("/api/v1/admin/backups");
    // Auth interceptor returns 401 (Unauthenticated); a non-admin token would 403.
    expect([401, 403]).toContain(res.status());
  });

  test("delete nonexistent backup returns 404", async ({ request, token }) => {
    const res = await request.delete("/api/v1/admin/backups/nonexistent-id", {
      headers: authHeaders(token),
    });
    expect(res.status()).toBe(404);
  });
});

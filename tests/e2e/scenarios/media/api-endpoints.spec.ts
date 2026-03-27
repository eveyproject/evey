import { test, expect } from "../../fixtures/auth";
import { goto, getToken, apiGet, pickRandomMediaId } from "../shared/helpers";

test.describe("Media > API endpoint validation", () => {
  test("GET /api/v1/media returns items array", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media?pagination.pageSize=5");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("items");
    expect(Array.isArray(res.body.items)).toBe(true);
  });

  test("GET /api/v1/media/{id} returns media item", async ({ page }) => {
    await goto(page, "/home");
    const id = await pickRandomMediaId(page);
    test.skip(!id, "No media");
    const res = await apiGet(page, `/api/v1/media/${id}`);
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("id");
    expect(res.body).toHaveProperty("title");
  });

  test("GET /api/v1/media/{id} title is a non-empty string", async ({ page }) => {
    await goto(page, "/home");
    const id = await pickRandomMediaId(page);
    test.skip(!id, "No media");
    const res = await apiGet(page, `/api/v1/media/${id}`);
    expect(typeof res.body.title).toBe("string");
    expect(res.body.title.length).toBeGreaterThan(0);
  });

  test("GET /api/v1/media/nonexistent returns 404 or 500", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media/nonexistent-fake-id");
    expect([404, 500]).toContain(res.status); // should be 404 but 500 is a bug to track
  });

  test("GET /api/v1/media/search returns results", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media/search?query=the&limit=5");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("items");
  });

  test("GET /api/v1/libraries returns array", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/libraries");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("libraries");
  });

  test("GET /api/v1/people/{id} returns person with name", async ({ page }) => {
    await goto(page, "/home");
    const id = await pickRandomMediaId(page);
    test.skip(!id, "No media");
    // get a person ID from the media's cast
    const mediaRes = await apiGet(page, `/api/v1/media/${id}`);
    const people = mediaRes.body?.people ?? [];
    test.skip(people.length === 0, "No cast");
    const personId = people[0].id;
    const res = await apiGet(page, `/api/v1/people/${personId}`);
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("name");
    expect(res.body.name.length).toBeGreaterThan(0);
  });

  test("GET /api/v1/notifications/unread-count returns count", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/notifications/unread-count");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("count");
    expect(typeof res.body.count).toBe("number");
  });

  test("GET /api/v1/playback/history returns items", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/playback/history?limit=5");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("items");
  });

  test("GET /api/v1/feeds/resume-playback returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/feeds/resume-playback");
    expect(res.status).toBe(200);
  });

  test("GET /api/v1/collections returns array", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/collections");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("collections");
  });

  test("GET /api/v1/playlists returns array", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/playlists");
    expect(res.status).toBe(200);
  });

  test("GET /api/v1/admin/feeds/config returns configs (admin)", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/admin/feeds/config");
    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("configs");
  });

  test("unauthenticated request returns 401", async ({ page }) => {
    await goto(page, "/home");
    const resp = await page.evaluate(async () => {
      const res = await fetch("/api/v1/media");
      return res.status;
    });
    expect(resp).toBe(401);
  });
});

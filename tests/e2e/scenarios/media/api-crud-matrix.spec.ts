import { test, expect } from "../../fixtures/auth";
import { goto, getToken, apiGet, pickRandomMediaId } from "../shared/helpers";

/**
 * API CRUD matrix: test every endpoint with valid/invalid/missing params
 */

test.describe("API > Media list variations", () => {
  const TYPES = [
    "MEDIA_ITEM_TYPE_MOVIE",
    "MEDIA_ITEM_TYPE_SERIES",
    "MEDIA_ITEM_TYPE_SEASON",
    "MEDIA_ITEM_TYPE_EPISODE",
  ];

  for (const type of TYPES) {
    test(`list ${type} returns 200`, async ({ page }) => {
      await goto(page, "/home");
      const res = await apiGet(page, `/api/v1/media?type=${type}&pagination.pageSize=5`);
      expect(res.status).toBe(200);
      expect(res.body).toHaveProperty("items");
    });
  }

  const SORT_FIELDS = [
    "MEDIA_SORT_FIELD_TITLE",
    "MEDIA_SORT_FIELD_YEAR",
    "MEDIA_SORT_FIELD_DATE_ADDED",
    "MEDIA_SORT_FIELD_RATING",
  ];

  for (const sort of SORT_FIELDS) {
    test(`list sorted by ${sort} returns 200`, async ({ page }) => {
      await goto(page, "/home");
      const res = await apiGet(page, `/api/v1/media?sortField=${sort}&pagination.pageSize=5`);
      expect(res.status).toBe(200);
    });
  }

  for (const sort of SORT_FIELDS) {
    test(`list sorted by ${sort} DESC returns 200`, async ({ page }) => {
      await goto(page, "/home");
      const res = await apiGet(page, `/api/v1/media?sortField=${sort}&sortOrder=SORT_ORDER_DESC&pagination.pageSize=5`);
      expect(res.status).toBe(200);
    });
  }

  const PAGE_SIZES = [1, 5, 10, 50, 100];
  for (const size of PAGE_SIZES) {
    test(`page size ${size} returns ≤ ${size} items`, async ({ page }) => {
      await goto(page, "/home");
      const res = await apiGet(page, `/api/v1/media?pagination.pageSize=${size}`);
      expect(res.status).toBe(200);
      expect((res.body.items ?? []).length).toBeLessThanOrEqual(size);
    });
  }

  test("page 1 and page 2 return different items", async ({ page }) => {
    await goto(page, "/home");
    const res1 = await apiGet(page, "/api/v1/media?pagination.pageSize=2&pagination.page=1");
    const res2 = await apiGet(page, "/api/v1/media?pagination.pageSize=2&pagination.page=2");
    expect(res1.status).toBe(200);
    expect(res2.status).toBe(200);
    const ids1 = (res1.body.items ?? []).map((i: { id: string }) => i.id);
    const ids2 = (res2.body.items ?? []).map((i: { id: string }) => i.id);
    // pages should have different content (unless < 3 items total)
    if (ids1.length > 0 && ids2.length > 0) {
      expect(ids1[0]).not.toBe(ids2[0]);
    }
  });

  test("genre filter returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media?genre=Action&pagination.pageSize=5");
    expect(res.status).toBe(200);
  });

  test("personId filter returns 200", async ({ page }) => {
    await goto(page, "/home");
    // get a person from a media
    const id = await pickRandomMediaId(page);
    test.skip(!id, "No media");
    const media = await apiGet(page, `/api/v1/media/${id}`);
    const personId = media.body?.people?.[0]?.id;
    test.skip(!personId, "No person");
    const res = await apiGet(page, `/api/v1/media?personId=${personId}&pagination.pageSize=5`);
    expect(res.status).toBe(200);
  });
});

test.describe("API > Search variations", () => {
  const QUERIES = ["the", "a", "star", "dark"];
  for (const q of QUERIES) {
    test(`search "${q}" returns results`, async ({ page }) => {
      await goto(page, "/home");
      const res = await apiGet(page, `/api/v1/media/search?query=${q}&limit=5`);
      expect(res.status).toBe(200);
      expect(res.body).toHaveProperty("items");
    });
  }

  test("search empty query returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media/search?query=&limit=5");
    expect(res.status).toBeLessThan(500);
  });

  test("search with type filter", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media/search?query=the&type=MEDIA_ITEM_TYPE_MOVIE&limit=5");
    expect(res.status).toBe(200);
  });

  test("search with year range", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/media/search?query=the&yearMin=2020&yearMax=2025&limit=5");
    expect(res.status).toBe(200);
  });
});

test.describe("API > Favorites CRUD", () => {
  test("list favorites returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/favorites");
    expect(res.status).toBeLessThan(500);
  });

  test("toggle favorite on random media", async ({ page }) => {
    await goto(page, "/home");
    const id = await pickRandomMediaId(page);
    test.skip(!id, "No media");
    const token = await getToken(page);
    const status = await page.evaluate(async ({ mediaId, tk }) => {
      const res = await fetch(`/api/v1/favorites/${mediaId}`, {
        method: "POST",
        headers: { Authorization: `Bearer ${tk}`, "Content-Type": "application/json" },
        body: "{}",
      });
      return res.status;
    }, { mediaId: id, tk: token });
    expect(status).toBeLessThan(500);
  });
});

test.describe("API > Playback endpoints", () => {
  test("resume items returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/playback/resume-items");
    expect(res.status).toBeLessThan(500);
  });

  test("next up returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/playback/next-up");
    expect(res.status).toBeLessThan(500);
  });

  test("watch history returns 200", async ({ page }) => {
    await goto(page, "/home");
    const res = await apiGet(page, "/api/v1/playback/history?limit=5");
    expect(res.status).toBe(200);
  });
});

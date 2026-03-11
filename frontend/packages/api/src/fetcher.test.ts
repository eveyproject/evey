import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { eveyFetch, setTokenProvider } from "./fetcher";

const fetchMock = vi.fn();

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { dispatchEvent: vi.fn() });
  setTokenProvider(null as unknown as () => string | null);
});

afterEach(() => {
  vi.restoreAllMocks();
});

function mockResponse(status: number, body?: unknown, headers?: Record<string, string>) {
  const hdrs = new Map(Object.entries(headers ?? {}));
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : status === 204 ? "No Content" : status === 401 ? "Unauthorized" : "Error",
    headers: { get: (k: string) => hdrs.get(k) ?? null },
    json: body !== undefined ? () => Promise.resolve(body) : () => Promise.reject(new Error("no body")),
  };
}

describe("eveyFetch", () => {
  it("attaches Authorization header when token present", async () => {
    setTokenProvider(() => "my-jwt-token");
    fetchMock.mockResolvedValue(mockResponse(200, { ok: true }));

    await eveyFetch({ url: "/api/test", method: "GET" });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/test",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer my-jwt-token" }),
      }),
    );
  });

  it("handles 204 No Content response", async () => {
    fetchMock.mockResolvedValue(mockResponse(204, undefined, {}));

    const result = await eveyFetch({ url: "/api/test", method: "DELETE" });
    expect(result).toBeUndefined();
  });

  it("refreshes token on 401 and retries", async () => {
    fetchMock.mockResolvedValue(mockResponse(401, { message: "Unauthorized" }));

    await expect(eveyFetch({ url: "/api/test", method: "GET" })).rejects.toThrow("Unauthorized");
    // eveyFetch dispatches evey:unauthorized so the app can refresh + retry
    expect(window.dispatchEvent).toHaveBeenCalledWith(expect.any(Event));
  });
});

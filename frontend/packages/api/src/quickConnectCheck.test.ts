import { describe, it, expect, vi } from "vitest";

const fetchMock = vi.fn(async (_req: unknown) => ({ authorized: false }));
vi.mock("./fetcher", () => ({ eveyFetch: (req: unknown) => fetchMock(req) }));

import { checkQuickConnect, QUICK_CONNECT_SECRET_HEADER } from "./quickConnectCheck";

describe("checkQuickConnect", () => {
  it("sends the secret as a header and keeps the URL bare", async () => {
    await checkQuickConnect("qc-code-1", "qc-secret-1");
    expect(fetchMock).toHaveBeenCalledWith({
      url: "/api/v1/auth/quick-connect/qc-code-1",
      method: "GET",
      headers: { [QUICK_CONNECT_SECRET_HEADER]: "qc-secret-1" },
    });
  });

  it("sends no header when the device holds no secret", async () => {
    await checkQuickConnect("qc-code-1", null);
    expect(fetchMock).toHaveBeenLastCalledWith({ url: "/api/v1/auth/quick-connect/qc-code-1", method: "GET", headers: undefined });
  });
});

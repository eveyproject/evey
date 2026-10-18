import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";

vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "jwt" }) }));

import { useAuthenticatedImage } from "./useAuthenticatedImage";

// A poster URL must not carry the token: it leaked into proxy logs and, the
// token changing at each refresh, defeated the image cache.
describe("useAuthenticatedImage (phone)", () => {
  it("keeps the token in the header and the poster URL stable", () => {
    const { result } = renderHook(() => useAuthenticatedImage("http://evey:9091/api/v1/media/m1/artwork/poster?w=400&v=1"));
    expect(result.current).toEqual({
      uri: "http://evey:9091/api/v1/media/m1/artwork/poster?w=400&v=1",
      headers: { Authorization: "Bearer jwt" },
    });
  });
});

import { describe, it, expect, vi } from "vitest";

vi.mock("i18next", () => ({ default: { t: (k: string) => `t:${k}` } }));

import { mutationErrorMessage } from "./mutationErrorMessage";

// httpStatus is only set once the server answered: without it the request
// never got there, and the honest message is "offline", not a generic error.
describe("mutationErrorMessage", () => {
  it("reads a missing httpStatus as offline and a server answer as a plain error", () => {
    expect(mutationErrorMessage(new Error("Failed to fetch"))).toBe("t:common.offline.description");
    expect(mutationErrorMessage(null)).toBe("t:common.offline.description");
    expect(mutationErrorMessage("boom")).toBe("t:common.offline.description");
    expect(mutationErrorMessage(Object.assign(new Error("bad"), { httpStatus: 500 }))).toBe("t:common.error");
    expect(mutationErrorMessage({ httpStatus: 403 })).toBe("t:common.error");
    expect(mutationErrorMessage({ httpStatus: "500" })).toBe("t:common.offline.description");
  });
});

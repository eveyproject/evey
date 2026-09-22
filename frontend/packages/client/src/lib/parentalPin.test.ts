import { beforeEach, describe, expect, it, vi } from "vitest";
import { verifyParentalPin } from "./parentalPin";

const verify = vi.fn();
vi.mock("@evey/api", () => ({ parentalServiceVerifyPin: (...args: unknown[]) => verify(...args) }));

describe("verifyParentalPin", () => {
  beforeEach(() => verify.mockReset());

  it("asks the profile being left first, because a PIN-less target says yes to anything", async () => {
    verify.mockResolvedValueOnce({ valid: true, pinToken: "proof-from-leaving" });
    const token = await verifyParentalPin({ targetProfileId: "target", leavingProfileId: "leaving", pin: "1234" });
    expect(verify).toHaveBeenCalledTimes(1);
    expect(verify).toHaveBeenCalledWith("leaving", { pin: "1234" });
    expect(token).toBe("proof-from-leaving");
  });

  it("falls through to the target when the profile being left refuses", async () => {
    verify.mockResolvedValueOnce({ valid: false }).mockResolvedValueOnce({ valid: true, pinToken: "proof" });
    expect(await verifyParentalPin({ targetProfileId: "target", leavingProfileId: "leaving", pin: "1234" })).toBe("proof");
    expect(verify).toHaveBeenNthCalledWith(2, "target", { pin: "1234" });
  });

  it("checks the one profile when there is nothing to leave (a content gate)", async () => {
    verify.mockResolvedValueOnce({ valid: true });
    expect(await verifyParentalPin({ targetProfileId: "target", pin: "1234" })).toBe("");
    expect(verify).toHaveBeenCalledTimes(1);
  });

  it("returns null when every profile refuses", async () => {
    verify.mockResolvedValue({ valid: false });
    expect(await verifyParentalPin({ targetProfileId: "target", leavingProfileId: "leaving", pin: "0000" })).toBeNull();
  });

  it("a refusal the server spelled out falls through to the next profile", async () => {
    verify.mockImplementationOnce(() => Promise.reject(Object.assign(new Error("no pin here"), { httpStatus: 400 })));
    verify.mockResolvedValueOnce({ valid: true, pinToken: "p" });
    expect(await verifyParentalPin({ targetProfileId: "target", leavingProfileId: "leaving", pin: "1234" })).toBe("p");
  });

  // A dead network used to read as "Incorrect PIN" on three of the four
  // clients: only the browser told them apart.
  it("rethrows a network failure instead of blaming the viewer", async () => {
    verify.mockImplementationOnce(() => Promise.reject(new Error("network down")));
    let caught: unknown;
    try {
      await verifyParentalPin({ targetProfileId: "target", pin: "1234" });
    } catch (err) {
      caught = err;
    }
    expect((caught as Error | undefined)?.message).toBe("network down");
  });
});

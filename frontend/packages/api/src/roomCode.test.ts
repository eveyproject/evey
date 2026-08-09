import { describe, it, expect } from "vitest";

import { ROOM_CODE_ALPHABET, ROOM_CODE_LENGTH, normalizeRoomCode, isValidRoomCode } from "./roomCode";

// The server owns this shape. The two-language drift guard lives on the GO
// side (internal/domain/syncplay/code_contract_test.go): frontend tests run in
// a container where only frontend/ is mounted, so a test reading model.go from
// here passes on the host and dies in CI.

describe("room code shape", () => {
  it("is six symbols over a 32-letter alphabet", () => {
    expect(ROOM_CODE_LENGTH).toBe(6);
    expect(ROOM_CODE_ALPHABET).toHaveLength(32);
    expect(ROOM_CODE_ALPHABET).not.toMatch(/[ILOU]/);
  });
});

describe("normalizeRoomCode", () => {
  it("folds case, separators and the characters the alphabet drops", () => {
    expect(normalizeRoomCode("abc123")).toBe("ABC123");
    expect(normalizeRoomCode(" abc 123 ")).toBe("ABC123");
    expect(normalizeRoomCode("ABC-123")).toBe("ABC123");
    expect(normalizeRoomCode("IL0O1")).toBe("11001");
  });

  it("keeps an old five-digit code intact", () => {
    expect(normalizeRoomCode("12345")).toBe("12345");
  });
});

describe("isValidRoomCode", () => {
  it("accepts a complete code, whatever the user typed around it", () => {
    expect(isValidRoomCode("abc-123")).toBe(true);
    expect(isValidRoomCode("ABC123")).toBe(true);
  });

  it("rejects an incomplete one", () => {
    expect(isValidRoomCode("ABC12")).toBe(false);
    expect(isValidRoomCode("")).toBe(false);
  });
});

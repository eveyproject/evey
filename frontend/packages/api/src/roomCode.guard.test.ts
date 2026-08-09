import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { describe, it, expect } from "vitest";

import { ROOM_CODE_ALPHABET, ROOM_CODE_LENGTH, normalizeRoomCode, isValidRoomCode } from "./roomCode";

// Drift guard across languages. The server MINTS room codes and the clients
// enter them; if the two alphabets or lengths ever disagree, every guest join
// fails - and it fails silently, as a plain "room not found", which reads as a
// server problem rather than a client one. Nothing else pins them together, so
// this test reads the Go source.
const goModel = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../internal/domain/syncplay/model.go",
);

describe("room code agrees with the server", () => {
  const src = readFileSync(goModel, "utf8");

  it("uses the same alphabet as GenerateCode", () => {
    const m = src.match(/const codeAlphabet = "([^"]+)"/);
    expect(m, "codeAlphabet not found in model.go - was it renamed?").not.toBeNull();
    expect(m![1]).toBe(ROOM_CODE_ALPHABET);
  });

  it("uses the same length as GenerateCode", () => {
    const m = src.match(/const codeLen = (\d+)/);
    expect(m, "codeLen not found in model.go - was it renamed?").not.toBeNull();
    expect(Number(m![1])).toBe(ROOM_CODE_LENGTH);
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

import { describe, expect, it } from "vitest";
import { isPlainEmail, passwordTooLong } from "./accountInput";

describe("isPlainEmail", () => {
  it.each(["a@b.com", "first.last+tag@example.org", "x@x"])("accepts %s", (v) => {
    expect(isPlainEmail(v)).toBe(true);
  });
  it.each(["alice", "a@", "@b.com", "a b@c.d", "Bob <a@b.com>", "bob.@example.com", "a..b@x.com", "a@.x.com", "a@x.com.", "a@x..com"])(
    "refuses %s",
    (v) => {
      expect(isPlainEmail(v)).toBe(false);
    },
  );
});

describe("passwordTooLong", () => {
  it("counts UTF-8 bytes after trimming, as the server does", () => {
    expect(passwordTooLong("a".repeat(72))).toBe(false);
    expect(passwordTooLong(`  ${"a".repeat(72)}  `)).toBe(false);
    expect(passwordTooLong("é".repeat(37))).toBe(true);
    expect(passwordTooLong("é".repeat(36))).toBe(false);
    expect(passwordTooLong("😀".repeat(18))).toBe(false);
    expect(passwordTooLong("😀".repeat(19))).toBe(true);
  });
});

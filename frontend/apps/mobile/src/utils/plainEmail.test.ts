import { describe, expect, it } from "vitest";
import { isPlainEmail } from "./plainEmail";

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
